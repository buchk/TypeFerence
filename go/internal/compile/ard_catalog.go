package compile

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"github.com/buchk/TypeFerence/go/internal/trust"
)

// ardBundle describes one compiled artifact directory as the catalog
// publishes it.
type ardBundle struct {
	target       string
	path         string // artifact directory beneath the target root
	displayName  string
	description  string
	capabilities []string
	version      string
	data         jsonx.Obj // identity members of the entry's data object
}

// writeArd emits the ARD catalog: the canonical source package plus one
// target-bundle entry per compiled artifact.
func (c *compilation) writeArd(root string, requested []Target, ard *ArdPublicationOptions, written *[]string) error {
	ardRoot := filepath.Join(root, "ard")
	if err := os.RemoveAll(ardRoot); err != nil {
		return resource.Errorf("Cannot reset target directory: %s", ardRoot)
	}
	if err := os.MkdirAll(ardRoot, 0o755); err != nil {
		return resource.Errorf("Cannot create target directory: %s", ardRoot)
	}
	signatures := map[string]string{}
	signatureKeys := []string{}
	if ard.TrustSignaturesPath != "" {
		sourceRoot, absErr := filepath.Abs(c.source)
		if absErr != nil {
			return resource.Errorf("Source directory not found: %s", c.source)
		}
		signaturePath, absErr := filepath.Abs(ard.TrustSignaturesPath)
		if absErr != nil {
			return resource.Errorf("Trust signatures file not found: %s", ard.TrustSignaturesPath)
		}
		if isBeneath(sourceRoot, signaturePath) {
			return resource.Errorf("Trust signatures file must be outside the source root to avoid a digest/signature cycle")
		}
		var err error
		signatures, signatureKeys, err = trust.LoadSignatures(ard.TrustSignaturesPath)
		if err != nil {
			return err
		}
	}
	var configuration *trust.Configuration
	if c.trust != nil {
		configuration = c.trust.Configuration
	}
	if len(signatures) > 0 && configuration == nil {
		return resource.Errorf("--trust-signatures requires a trust configuration")
	}
	if ard.AllowUnsignedTrust && configuration == nil {
		return resource.Errorf("--allow-unsigned-trust requires a trust configuration")
	}
	bundles := []ardBundle{}
	for _, target := range requested {
		switch target {
		case Neutral:
			for _, agent := range c.agents {
				capabilities := make([]string, len(agent.Skills))
				for i, skill := range agent.Skills {
					capabilities[i] = skill.DispatchName
				}
				sort.Strings(capabilities)
				bundles = append(bundles, ardBundle{
					target:       target.String(),
					path:         resolve.Leaf(agent.ID),
					displayName:  agent.DisplayName + " (" + target.String() + ")",
					description:  "Precompiled " + target.String() + " artifact bundle. " + agent.Description,
					capabilities: capabilities,
					version:      agent.ID[strings.LastIndex(agent.ID, "@")+1:],
					data:         jsonx.Obj{{K: "agentId", V: jsonx.Str(agent.ID)}},
				})
			}
		case AgentPlugin:
			for _, artifact := range artifactsOf(c.plugins) {
				capabilities := make([]string, len(artifact.plan.Skills))
				for i, skill := range artifact.plan.Skills {
					capabilities[i] = skillName(skill)
				}
				sort.Strings(capabilities)
				bundles = append(bundles, ardBundle{
					target:       target.String(),
					path:         artifact.dir,
					displayName:  artifact.dir + " (" + target.String() + ")",
					description:  "Precompiled " + target.String() + " artifact bundle. " + artifact.plan.Description,
					capabilities: capabilities,
					version:      artifact.plan.Version,
					data: jsonx.Obj{
						{K: "pluginId", V: jsonx.Str(artifact.plan.ID)},
						{K: "mode", V: jsonx.Str(artifact.mode)},
					},
				})
			}
		}
	}
	return writeArdCatalog(ardRoot, c.source, root, bundles, ard.PublisherDomain,
		configuration, signatures, signatureKeys, ard.AllowUnsignedTrust, written)
}

func writeArdCatalog(
	ardRoot, source, outputRoot string,
	bundles []ardBundle,
	publisherDomain string,
	configuration *trust.Configuration,
	signatures map[string]string,
	signatureKeys []string,
	allowUnsignedTrust bool,
	written *[]string,
) error {
	if len(publisherDomain) == 0 || len(publisherDomain) > 253 || !publisherDomainBody.MatchString(publisherDomain) {
		return resource.Errorf("Invalid ARD publisher domain: %s", publisherDomain)
	}
	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return resource.Errorf("Source directory not found: %s", source)
	}
	sourceName := urnSegment(filepath.Base(strings.TrimRight(sourceAbs, `\/`)))
	sourceVersion := "1.0.0"
	project, projErr := resource.LoadProject(source)
	if projErr != nil {
		return projErr
	}
	if project != nil {
		if strings.TrimSpace(project.Name) != "" {
			sourceName = urnSegment(project.Name)
		}
		if strings.TrimSpace(project.Version) != "" {
			sourceVersion = project.Version
		}
	}
	sourceIdentifier := "urn:air:" + publisherDomain + ":typeference:source:" + sourceName
	sourceHash, err := HashSource(source)
	if err != nil {
		return err
	}
	sourceDigest := "sha256:" + sourceHash

	sourceFiles, err := sourcePackageFiles(source)
	if err != nil {
		return err
	}
	signedIdentifiers := map[string]bool{}
	entries := jsonx.Arr{}

	sourceBase := jsonx.Obj{
		{K: "identifier", V: jsonx.Str(sourceIdentifier)},
		{K: "displayName", V: jsonx.Str("TypeFerence source package: " + sourceName)},
		{K: "type", V: jsonx.Str("application/vnd.typeference.source-package+json")},
		{K: "description", V: jsonx.Str("Canonical typed source package for validation, audit, and reproducible compilation.")},
		{K: "version", V: jsonx.Str(sourceVersion)},
		{K: "data", V: jsonx.Obj{
			{K: "schemaVersion", V: jsonx.Num("1")},
			{K: "digest", V: jsonx.Str(sourceDigest)},
			{K: "files", V: sourceFiles},
		}},
		{K: "metadata", V: jsonx.Obj{
			{K: "generatedBy", V: jsonx.Str("TypeFerence")},
			{K: "role", V: jsonx.Str("canonical-source")},
		}},
	}
	if configuration == nil || configuration.Source == nil {
		entries = append(entries, sourceBase)
	} else {
		if err := trust.ValidateIdentityForPublisher(
			configuration.Source.Identity, configuration.Source.IdentityType, publisherDomain, "source.identity"); err != nil {
			return err
		}
		manifest, err := buildTrustManifest(
			&configuration.Source.Profile,
			configuration.Source.Identity,
			nil,
			sourceDigest,
			signatures[sourceIdentifier],
			sourceIdentifier,
			allowUnsignedTrust)
		if err != nil {
			return err
		}
		if _, ok := signatures[sourceIdentifier]; ok {
			signedIdentifiers[sourceIdentifier] = true
		}
		entries = append(entries, append(sourceBase, jsonx.Member{K: "trustManifest", V: manifest}))
	}

	for _, bundle := range bundles {
		artifactRoot := filepath.Join(outputRoot, bundle.target, filepath.FromSlash(bundle.path))
		identifier := "urn:air:" + publisherDomain + ":typeference:" + bundle.target + ":" + bundle.path
		files, err := packageFiles(artifactRoot)
		if err != nil {
			return err
		}
		data := jsonx.Obj{
			{K: "schemaVersion", V: jsonx.Num("1")},
			{K: "target", V: jsonx.Str(bundle.target)},
		}
		data = append(data, bundle.data...)
		data = append(data, jsonx.Member{K: "files", V: files})
		targetBase := jsonx.Obj{
			{K: "identifier", V: jsonx.Str(identifier)},
			{K: "displayName", V: jsonx.Str(bundle.displayName)},
			{K: "type", V: jsonx.Str("application/vnd.typeference.target-bundle+json")},
			{K: "description", V: jsonx.Str(bundle.description)},
			{K: "capabilities", V: stringArr(bundle.capabilities)},
			{K: "version", V: jsonx.Str(bundle.version)},
			{K: "data", V: data},
			{K: "metadata", V: jsonx.Obj{
				{K: "generatedBy", V: jsonx.Str("TypeFerence")},
				{K: "sourceDigest", V: jsonx.Str(sourceDigest)},
				{K: "sourceIdentifier", V: jsonx.Str(sourceIdentifier)},
				{K: "target", V: jsonx.Str(bundle.target)},
			}},
		}
		if configuration == nil || configuration.Bundles == nil {
			manifest := jsonx.Obj{
				{K: "identity", V: jsonx.Str("https://" + publisherDomain)},
				{K: "identityType", V: jsonx.Str("https")},
				{K: "provenance", V: jsonx.Arr{jsonx.Obj{
					{K: "relation", V: jsonx.Str("derivedFrom")},
					{K: "sourceId", V: jsonx.Str(sourceIdentifier)},
					{K: "sourceDigest", V: jsonx.Str(sourceDigest)},
				}}},
			}
			entries = append(entries, append(targetBase, jsonx.Member{K: "trustManifest", V: manifest}))
			continue
		}
		identity := trust.ExpandIdentity(
			configuration.Bundles.IdentityTemplate, publisherDomain, bundle.target, bundle.path, bundle.version)
		if err := trust.ValidateIdentityForPublisher(
			identity, configuration.Bundles.IdentityType, publisherDomain, "bundles.identityTemplate"); err != nil {
			return err
		}
		artifactHash, err := HashDirectory(artifactRoot)
		if err != nil {
			return err
		}
		manifest, err := buildTrustManifest(
			&configuration.Bundles.Profile,
			identity,
			[]trust.ProvenanceLink{{Relation: "derivedFrom", SourceID: sourceIdentifier, SourceDigest: sourceDigest}},
			"sha256:"+artifactHash,
			signatures[identifier],
			identifier,
			allowUnsignedTrust)
		if err != nil {
			return err
		}
		if _, ok := signatures[identifier]; ok {
			signedIdentifiers[identifier] = true
		}
		entries = append(entries, append(targetBase, jsonx.Member{K: "trustManifest", V: manifest}))
	}

	for _, key := range signatureKeys {
		if !signedIdentifiers[key] {
			return resource.Errorf("Trust signature identifier does not match a configured catalog entry: %s", key)
		}
	}

	catalog := jsonx.Obj{
		{K: "specVersion", V: jsonx.Str("1.0")},
		{K: "host", V: jsonx.Obj{
			{K: "displayName", V: jsonx.Str(publisherDomain)},
			{K: "identifier", V: jsonx.Str(publisherDomain)},
		}},
		{K: "entries", V: entries},
	}
	return writeFile(filepath.Join(ardRoot, "ai-catalog.json"), jsonx.Indented(catalog)+"\n", written)
}

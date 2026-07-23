package compile

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resolve"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"github.com/buchk/TypeFerence/go/internal/trust"
)

var (
	publisherDomainBody = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	urnUnsafe           = regexp.MustCompile(`[^a-z0-9-]+`)
)

func writeArdCatalog(
	ardRoot, source, outputRoot string,
	agents []*resolve.ResolvedAgent,
	targets []Target,
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
	sourceIdentifier := "urn:air:" + publisherDomain + ":typeference:source:" + sourceName
	sourceHash, err := HashDirectory(source)
	if err != nil {
		return err
	}
	sourceDigest := "sha256:" + sourceHash

	sourceFiles, err := packageFiles(source)
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
		{K: "version", V: jsonx.Str("1.0.0")},
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

	for _, target := range targets {
		for _, agent := range agents {
			targetName := target.String()
			slug := resolve.Leaf(agent.ID)
			agentRoot := filepath.Join(outputRoot, targetName, slug)
			identifier := "urn:air:" + publisherDomain + ":typeference:" + targetName + ":" + slug
			version := agent.ID[strings.LastIndex(agent.ID, "@")+1:]

			capabilities := make([]string, len(agent.Skills))
			for i, skill := range agent.Skills {
				capabilities[i] = skill.DispatchName
			}
			sort.Strings(capabilities)

			files, err := packageFiles(agentRoot)
			if err != nil {
				return err
			}
			targetBase := jsonx.Obj{
				{K: "identifier", V: jsonx.Str(identifier)},
				{K: "displayName", V: jsonx.Str(agent.DisplayName + " (" + targetName + ")")},
				{K: "type", V: jsonx.Str("application/vnd.typeference.target-bundle+json")},
				{K: "description", V: jsonx.Str("Precompiled " + targetName + " artifact bundle. " + agent.Description)},
				{K: "capabilities", V: stringArr(capabilities)},
				{K: "version", V: jsonx.Str(version)},
				{K: "data", V: jsonx.Obj{
					{K: "schemaVersion", V: jsonx.Num("1")},
					{K: "target", V: jsonx.Str(targetName)},
					{K: "agentId", V: jsonx.Str(agent.ID)},
					{K: "files", V: files},
				}},
				{K: "metadata", V: jsonx.Obj{
					{K: "generatedBy", V: jsonx.Str("TypeFerence")},
					{K: "sourceDigest", V: jsonx.Str(sourceDigest)},
					{K: "sourceIdentifier", V: jsonx.Str(sourceIdentifier)},
					{K: "target", V: jsonx.Str(targetName)},
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
			} else {
				identity := trust.ExpandIdentity(
					configuration.Bundles.IdentityTemplate, publisherDomain, targetName, slug, version)
				if err := trust.ValidateIdentityForPublisher(
					identity, configuration.Bundles.IdentityType, publisherDomain, "bundles.identityTemplate"); err != nil {
					return err
				}
				agentHash, err := HashDirectory(agentRoot)
				if err != nil {
					return err
				}
				manifest, err := buildTrustManifest(
					&configuration.Bundles.Profile,
					identity,
					[]trust.ProvenanceLink{{Relation: "derivedFrom", SourceID: sourceIdentifier, SourceDigest: sourceDigest}},
					"sha256:"+agentHash,
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
		}
	}

	// Callable-resource cards: the fully-assembled invocation contract for each
	// agent's exposed capabilities (ADR-0018). Emitted only when an agent
	// exposes something, so agents with no exposed capability add no entry and
	// their catalog is unchanged.
	for _, agent := range agents {
		exposed := agent.ExposedSkills()
		if len(exposed) == 0 {
			continue
		}
		slug := resolve.Leaf(agent.ID)
		identifier := "urn:air:" + publisherDomain + ":typeference:callable:" + slug
		version := agent.ID[strings.LastIndex(agent.ID, "@")+1:]

		byName := map[string]resolve.ResolvedSkill{}
		toolNames := make([]string, 0, len(exposed))
		for _, skill := range exposed {
			byName[skill.DispatchName] = skill
			toolNames = append(toolNames, skill.DispatchName)
		}
		sort.Strings(toolNames)
		tools := jsonx.Arr{}
		for _, name := range toolNames {
			skill := byName[name]
			tools = append(tools, jsonx.Obj{
				{K: "name", V: jsonx.Str(skill.DispatchName)},
				{K: "description", V: jsonx.Str(skill.Description)},
				{K: "inputSchema", V: jsonx.Str(skill.InputSchema)},
				{K: "outputSchema", V: jsonx.Str(skill.OutputSchema)},
				// A callable card is the agent-to-agent surface, so it renders the
				// a2a variant when the skill declares one (ADR-0012, ADR-0018).
				{K: "instructionsTemplate", V: jsonx.Str(skill.InstructionsFor("a2a"))},
			})
		}
		manifest := jsonx.Obj{
			{K: "identity", V: jsonx.Str("https://" + publisherDomain)},
			{K: "identityType", V: jsonx.Str("https")},
			{K: "provenance", V: jsonx.Arr{jsonx.Obj{
				{K: "relation", V: jsonx.Str("derivedFrom")},
				{K: "sourceId", V: jsonx.Str(sourceIdentifier)},
				{K: "sourceDigest", V: jsonx.Str(sourceDigest)},
			}}},
		}
		entries = append(entries, jsonx.Obj{
			{K: "identifier", V: jsonx.Str(identifier)},
			{K: "displayName", V: jsonx.Str(agent.DisplayName + " (callable)")},
			{K: "type", V: jsonx.Str("application/vnd.typeference.callable-card+json")},
			{K: "description", V: jsonx.Str("Callable resource card: exposed capabilities of " + agent.DisplayName + ".")},
			{K: "capabilities", V: stringArr(toolNames)},
			{K: "version", V: jsonx.Str(version)},
			{K: "data", V: jsonx.Obj{
				{K: "schemaVersion", V: jsonx.Num("1")},
				{K: "agentId", V: jsonx.Str(agent.ID)},
				{K: "tools", V: tools},
			}},
			{K: "metadata", V: jsonx.Obj{
				{K: "generatedBy", V: jsonx.Str("TypeFerence")},
				{K: "role", V: jsonx.Str("callable-resource")},
				{K: "sourceDigest", V: jsonx.Str(sourceDigest)},
				{K: "sourceIdentifier", V: jsonx.Str(sourceIdentifier)},
			}},
			{K: "trustManifest", V: manifest},
		})
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

// buildTrustManifest assembles a canonically ordered trust manifest. Fails
// closed when the profile requires a signature and none is present, unless
// unsigned staging output was explicitly allowed.
func buildTrustManifest(
	profile *trust.Profile,
	identity string,
	compilerProvenance []trust.ProvenanceLink,
	artifactDigest string,
	signature string,
	catalogIdentifier string,
	allowUnsignedTrust bool,
) (jsonx.Value, error) {
	if profile.SignatureIntent != nil && profile.SignatureIntent.Required && signature == "" && !allowUnsignedTrust {
		return nil, resource.Errorf("Trust signature is required for catalog entry: %s", catalogIdentifier)
	}
	digestKey := trust.MetadataPrefix + ".artifactDigest"
	intentKey := trust.MetadataPrefix + ".signatureIntent"
	if profile.Metadata.Has(digestKey) || profile.Metadata.Has(intentKey) {
		return nil, resource.Errorf("Trust metadata cannot override TypeFerence-managed keys for catalog entry: %s", catalogIdentifier)
	}

	metadata := sortedPairs{}
	for _, key := range profile.Metadata.Keys() {
		metadata.add(key, metadataValueJSON(profile.Metadata.Get(key)))
	}
	digestObj := sortedPairs{}
	digestObj.add("digest", jsonx.Str(artifactDigest))
	digestObj.add("scheme", jsonx.Str("typeference-directory-v1"))
	metadata.add(digestKey, digestObj.obj())
	if profile.SignatureIntent != nil {
		intent := sortedPairs{}
		intent.add("required", jsonx.Bool(profile.SignatureIntent.Required))
		intent.add("status", jsonx.Str("external"))
		if strings.TrimSpace(profile.SignatureIntent.Algorithm) != "" {
			intent.add("algorithm", jsonx.Str(profile.SignatureIntent.Algorithm))
		}
		if strings.TrimSpace(profile.SignatureIntent.KeyRef) != "" {
			intent.add("keyRef", jsonx.Str(profile.SignatureIntent.KeyRef))
		}
		metadata.add(intentKey, intent.obj())
	}

	manifest := sortedPairs{}
	manifest.add("identity", jsonx.Str(identity))
	manifest.add("metadata", metadata.obj())
	if strings.TrimSpace(profile.IdentityType) != "" {
		manifest.add("identityType", jsonx.Str(profile.IdentityType))
	}
	if profile.TrustSchema != nil {
		schema := sortedPairs{}
		schema.add("identifier", jsonx.Str(profile.TrustSchema.Identifier))
		schema.add("version", jsonx.Str(profile.TrustSchema.Version))
		if profile.TrustSchema.GovernanceURI != "" {
			schema.add("governanceUri", jsonx.Str(profile.TrustSchema.GovernanceURI))
		}
		if len(profile.TrustSchema.VerificationMethods) > 0 {
			schema.add("verificationMethods", stringArr(profile.TrustSchema.VerificationMethods))
		}
		manifest.add("trustSchema", schema.obj())
	}
	if len(profile.Attestations) > 0 {
		attestations := jsonx.Arr{}
		for _, a := range profile.Attestations {
			pairs := sortedPairs{}
			pairs.add("type", jsonx.Str(a.Type))
			pairs.add("uri", jsonx.Str(a.URI))
			if a.Digest != "" {
				pairs.add("digest", jsonx.Str(a.Digest))
			}
			if a.Size != nil {
				pairs.add("size", jsonx.Int(*a.Size))
			}
			if a.Description != "" {
				pairs.add("description", jsonx.Str(a.Description))
			}
			attestations = append(attestations, pairs.obj())
		}
		manifest.add("attestations", attestations)
	}
	provenance := jsonx.Arr{}
	for _, link := range append(append([]trust.ProvenanceLink{}, compilerProvenance...), profile.Provenance...) {
		pairs := sortedPairs{}
		pairs.add("relation", jsonx.Str(link.Relation))
		pairs.add("sourceId", jsonx.Str(link.SourceID))
		if link.SourceDigest != "" {
			pairs.add("sourceDigest", jsonx.Str(link.SourceDigest))
		}
		if link.RegistryURI != "" {
			pairs.add("registryUri", jsonx.Str(link.RegistryURI))
		}
		if link.StatementURI != "" {
			pairs.add("statementUri", jsonx.Str(link.StatementURI))
		}
		if link.SignatureRef != "" {
			pairs.add("signatureRef", jsonx.Str(link.SignatureRef))
		}
		provenance = append(provenance, pairs.obj())
	}
	if len(provenance) > 0 {
		manifest.add("provenance", provenance)
	}
	if signature != "" {
		manifest.add("signature", jsonx.Str(signature))
	}
	return manifest.obj(), nil
}

func metadataValueJSON(value trust.MetadataValue) jsonx.Value {
	switch v := value.(type) {
	case nil:
		return jsonx.Null{}
	case string:
		return jsonx.Str(v)
	case bool:
		return jsonx.Bool(v)
	case int64:
		return jsonx.Int(v)
	case trust.MetadataMap:
		pairs := sortedPairs{}
		for _, key := range v.Keys() {
			pairs.add(key, metadataValueJSON(v.Get(key)))
		}
		return pairs.obj()
	case trust.MetadataList:
		arr := jsonx.Arr{}
		for _, item := range v {
			arr = append(arr, metadataValueJSON(item))
		}
		return arr
	default:
		return jsonx.Null{}
	}
}

// sortedPairs builds a JSON object whose members are emitted in canonical
// key order regardless of insertion order.
type sortedPairs struct{ members jsonx.Obj }

func (p *sortedPairs) add(key string, value jsonx.Value) {
	p.members = append(p.members, jsonx.Member{K: key, V: value})
}

func (p *sortedPairs) obj() jsonx.Obj {
	sorted := append(jsonx.Obj{}, p.members...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].K < sorted[j].K })
	return sorted
}

func packageFiles(root string) (jsonx.Arr, error) {
	files, err := relativeFiles(root)
	if err != nil {
		return nil, err
	}
	arr := jsonx.Arr{}
	for _, rel := range files {
		content, readErr := readTextFile(filepath.Join(root, filepath.FromSlash(rel)))
		if readErr != nil {
			return nil, readErr
		}
		arr = append(arr, jsonx.Obj{
			{K: "path", V: jsonx.Str(rel)},
			{K: "mediaType", V: jsonx.Str(mediaType(rel))},
			{K: "content", V: jsonx.Str(content)},
		})
	}
	return arr, nil
}

func mediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".toml":
		return "application/toml"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".md", ".mdc":
		return "text/markdown"
	default:
		return "text/plain"
	}
}

func urnSegment(value string) string {
	segment := strings.Trim(urnUnsafe.ReplaceAllString(strings.ToLower(value), "-"), "-")
	if segment == "" {
		return "package"
	}
	return segment
}

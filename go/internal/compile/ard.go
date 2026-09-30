package compile

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/packages"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"github.com/buchk/TypeFerence/go/internal/trust"
)

var (
	publisherDomainBody = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	urnUnsafe           = regexp.MustCompile(`[^a-z0-9-]+`)
)

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

func sourcePackageFiles(root string) (jsonx.Arr, error) {
	files, err := packages.SourceFiles(root)
	if err != nil {
		return nil, err
	}
	arr := jsonx.Arr{}
	for _, file := range files {
		arr = append(arr, jsonx.Obj{
			{K: "path", V: jsonx.Str(file.Path)},
			{K: "mediaType", V: jsonx.Str(mediaType(file.Path))},
			{K: "content", V: jsonx.Str(file.Content)},
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

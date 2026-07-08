// Package trust loads and validates the optional typeference.trust.yaml
// configuration and external detached-JWS signature maps
// (docs/specification.md, "Trust metadata compilation"). TypeFerence never
// signs anything; it imports externally produced signatures, and the
// signature map must live outside the source root.
package trust

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/buchk/TypeFerence/go/internal/jsonx"
	"github.com/buchk/TypeFerence/go/internal/resource"
	"gopkg.in/yaml.v3"
)

// DefaultFileName is the conventional trust configuration file name.
const DefaultFileName = "typeference.trust.yaml"

// MetadataPrefix scopes TypeFerence-managed trust metadata keys.
const MetadataPrefix = "com.github.buchk.typeference"

// Schema describes the trust schema a profile claims to follow.
type Schema struct {
	Identifier          string
	Version             string
	GovernanceURI       string
	VerificationMethods []string
}

// Attestation is an externally hosted trust attestation.
type Attestation struct {
	Type        string
	URI         string
	Digest      string
	Size        *int64
	Description string
}

// ProvenanceLink is one provenance edge in a trust manifest.
type ProvenanceLink struct {
	Relation     string
	SourceID     string
	SourceDigest string
	RegistryURI  string
	StatementURI string
	SignatureRef string
}

// SignatureIntent declares whether a catalog entry must carry a signature.
type SignatureIntent struct {
	Algorithm string
	KeyRef    string
	Required  bool
}

// Profile is the shared shape of source and bundle trust profiles.
type Profile struct {
	IdentityType    string
	TrustSchema     *Schema
	Attestations    []Attestation
	Provenance      []ProvenanceLink
	Metadata        MetadataMap
	SignatureIntent *SignatureIntent
}

// SourceProfile configures trust for the canonical source package.
type SourceProfile struct {
	Profile
	Identity string
}

// BundleProfile configures trust for compiled target bundles.
type BundleProfile struct {
	Profile
	IdentityTemplate string
}

// Configuration is a parsed typeference.trust.yaml.
type Configuration struct {
	SchemaVersion int
	Source        *SourceProfile
	Bundles       *BundleProfile
}

// Loaded pairs a configuration with the path it was read from.
type Loaded struct {
	Configuration *Configuration
	Path          string
}

// MetadataValue is a canonicalized trust metadata value: string, bool(never —
// YAML scalars canonicalize to strings), MetadataMap, MetadataList, or nil.
type MetadataValue any

// MetadataMap is a canonically ordered metadata object.
type MetadataMap struct {
	keys   []string
	values map[string]MetadataValue
}

// MetadataList is a metadata sequence.
type MetadataList []MetadataValue

// NewMetadataMap returns an empty metadata map.
func NewMetadataMap() MetadataMap {
	return MetadataMap{values: map[string]MetadataValue{}}
}

// Set inserts or replaces a key.
func (m *MetadataMap) Set(key string, value MetadataValue) {
	if m.values == nil {
		m.values = map[string]MetadataValue{}
	}
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
		sort.Strings(m.keys)
	}
	m.values[key] = value
}

// Has reports whether key is present.
func (m *MetadataMap) Has(key string) bool {
	_, ok := m.values[key]
	return ok
}

// Keys returns keys in canonical order.
func (m *MetadataMap) Keys() []string { return m.keys }

// Get returns the value for key.
func (m *MetadataMap) Get(key string) MetadataValue { return m.values[key] }

// Len returns the number of entries.
func (m *MetadataMap) Len() int { return len(m.keys) }

var (
	didPattern       = regexp.MustCompile(`^did:[a-z0-9]+:[A-Za-z0-9._:%-]+(?::[A-Za-z0-9._:%-]+)*(?:/[^?#]*)?(?:\?[^#]*)?(?:#.*)?$`)
	digestPattern    = regexp.MustCompile(`^(?:sha256:[0-9a-f]{64}|sha384:[0-9a-f]{96}|sha512:[0-9a-f]{128})$`)
	base64URLPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	placeholder      = regexp.MustCompile(`\{([^{}]+)\}`)
	metadataKey      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

var knownIdentityTypes = map[string]bool{"did": true, "dns": true, "https": true, "spiffe": true}

// Load reads the trust configuration for a source directory. Returns nil when
// no configuration exists and none was explicitly requested.
func Load(sourceDir, configuredPath string) (*Loaded, error) {
	root, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, resource.Errorf("Source directory not found: %s", sourceDir)
	}
	path := filepath.Join(root, DefaultFileName)
	if configuredPath != "" {
		path, err = filepath.Abs(configuredPath)
		if err != nil {
			return nil, resource.Errorf("Trust configuration not found: %s", configuredPath)
		}
	}
	if configuredPath == "" {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, nil
		}
	}
	if err := ensureBeneathRoot(root, path); err != nil {
		return nil, err
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, resource.Errorf("Trust configuration not found: %s", path)
	}
	configuration, parseErr := parseConfiguration(strings.TrimPrefix(string(raw), string(rune(0xFEFF))))
	if parseErr != nil {
		return nil, resource.Errorf("%s: invalid trust configuration: %s", path, parseErr)
	}
	if configuration == nil {
		return nil, resource.Errorf("Empty trust configuration: %s", path)
	}
	if err := validate(configuration, path); err != nil {
		return nil, err
	}
	return &Loaded{Configuration: configuration, Path: path}, nil
}

func ensureBeneathRoot(root, path string) error {
	prefix := root + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix)) {
			return resource.Errorf("Trust configuration must be beneath source root: %s", path)
		}
		return nil
	}
	if !strings.HasPrefix(path, prefix) {
		return resource.Errorf("Trust configuration must be beneath source root: %s", path)
	}
	return nil
}

// LoadSignatures reads a detached-JWS signature map keyed by catalog
// identifier.
func LoadSignatures(path string) (map[string]string, []string, error) {
	full, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, resource.Errorf("Trust signatures file not found: %s", path)
	}
	raw, readErr := os.ReadFile(full)
	if readErr != nil {
		return nil, nil, resource.Errorf("Trust signatures file not found: %s", full)
	}
	parsed, parseErr := jsonx.Parse(string(raw))
	if parseErr != nil {
		return nil, nil, resource.Errorf("%s: invalid trust signatures JSON: %s", full, parseErr)
	}
	obj, ok := parsed.(jsonx.Obj)
	if !ok {
		return nil, nil, resource.Errorf("Trust signatures file must be a JSON object keyed by catalog identifier")
	}
	result := map[string]string{}
	keys := []string{}
	for _, member := range obj {
		str, isString := member.V.(jsonx.Str)
		if !isString {
			return nil, nil, resource.Errorf("Trust signature for %s must be a string", member.K)
		}
		if err := validateDetachedJWS(string(str), member.K); err != nil {
			return nil, nil, err
		}
		if _, exists := result[member.K]; exists {
			return nil, nil, resource.Errorf("Duplicate trust signature identifier: %s", member.K)
		}
		result[member.K] = string(str)
		keys = append(keys, member.K)
	}
	sort.Strings(keys)
	return result, keys, nil
}

func validateDetachedJWS(value, identifier string) error {
	segments := strings.Split(value, ".")
	if len(segments) != 3 || segments[0] == "" || segments[1] != "" || segments[2] == "" ||
		!base64URLPattern.MatchString(segments[0]) || !base64URLPattern.MatchString(segments[2]) {
		return resource.Errorf("Trust signature for %s must be compact detached JWS (protected..signature)", identifier)
	}
	return nil
}

// ExpandIdentity substitutes identity template placeholders.
func ExpandIdentity(template, publisher, target, agent, version string) string {
	r := strings.NewReplacer("{publisher}", publisher, "{target}", target, "{agent}", agent, "{version}", version)
	return r.Replace(template)
}

// ValidateIdentityForPublisher checks that an identity's trust domain aligns
// with the ARD publisher domain. Identities must use ASCII (already
// punycoded) authorities per the spec's canonical-ASCII rule.
func ValidateIdentityForPublisher(identity, identityType, publisherDomain, field string) error {
	if err := ValidateIdentity(identity, identityType, field); err != nil {
		return err
	}
	var domain string
	switch {
	case strings.HasPrefix(identity, "did:web:"):
		rest := identity[len("did:web:"):]
		rest = strings.FieldsFunc(rest, func(r rune) bool {
			return r == ':' || r == '/' || r == '?' || r == '#'
		})[0]
		unescaped, err := url.PathUnescape(rest)
		if err != nil {
			unescaped = rest
		}
		domain = strings.Split(unescaped, ":")[0]
	case strings.HasPrefix(identity, "dns:"):
		rest := identity[strings.Index(identity, ":")+1:]
		rest = strings.TrimLeft(rest, "/")
		domain = strings.FieldsFunc(rest, func(r rune) bool {
			return r == '/' || r == '?' || r == '#'
		})[0]
	default:
		if u, err := url.Parse(identity); err == nil {
			domain = u.Hostname()
		}
	}
	if strings.TrimSpace(domain) == "" {
		return resource.Errorf("%s does not expose an authority or trust domain that can align with ARD publisher domain '%s'", field, publisherDomain)
	}
	if !strings.EqualFold(domain, publisherDomain) {
		return resource.Errorf("%s domain '%s' does not align with ARD publisher domain '%s'", field, domain, publisherDomain)
	}
	return nil
}

// ValidateIdentity checks URI shape, DID syntax, and identity-type/scheme
// consistency.
func ValidateIdentity(identity, identityType, field string) error {
	// Spec ("Canonical text and ordering"): identities must be ASCII so
	// domain alignment does not depend on an implementation's IDN handling.
	for _, r := range identity {
		if r > 0x7F {
			return resource.Errorf("%s must be ASCII; encode internationalized authorities as punycode", field)
		}
	}
	if err := validateURI(identity, field, false); err != nil {
		return err
	}
	if strings.HasPrefix(identity, "did:") && !didPattern.MatchString(identity) {
		return resource.Errorf("%s is not valid DID syntax", field)
	}
	expected := ""
	switch {
	case strings.HasPrefix(identity, "did:"):
		expected = "did"
	case strings.HasPrefix(identity, "spiffe://"):
		expected = "spiffe"
	case strings.HasPrefix(identity, "https://"):
		expected = "https"
	case strings.HasPrefix(identity, "dns:"):
		expected = "dns"
	}
	if identityType != "" && knownIdentityTypes[identityType] && expected != "" && identityType != expected {
		return resource.Errorf("%s scheme does not match identityType '%s'", field, identityType)
	}
	return nil
}

func validateURI(value, field string, allowData bool) error {
	u, err := url.Parse(value)
	if err != nil || !u.IsAbs() {
		return resource.Errorf("%s must be an absolute URI", field)
	}
	scheme := strings.ToLower(u.Scheme)
	if !allowData && scheme == "data" {
		return resource.Errorf("%s cannot be a data URI", field)
	}
	if allowData && scheme != "https" && scheme != "data" {
		return resource.Errorf("%s must use https or data", field)
	}
	return nil
}

func validate(configuration *Configuration, file string) error {
	if configuration.SchemaVersion != 1 {
		return resource.Errorf("%s: schemaVersion must be 1", file)
	}
	if configuration.Source == nil && configuration.Bundles == nil {
		return resource.Errorf("%s: at least one of source or bundles is required", file)
	}
	if configuration.Source != nil {
		if strings.TrimSpace(configuration.Source.Identity) == "" {
			return resource.Errorf("%s: source.identity is required", file)
		}
		if err := ValidateIdentity(configuration.Source.Identity, configuration.Source.IdentityType, "source.identity"); err != nil {
			return err
		}
		if err := validateProfile(&configuration.Source.Profile, "source"); err != nil {
			return err
		}
	}
	if configuration.Bundles != nil {
		template := configuration.Bundles.IdentityTemplate
		if strings.TrimSpace(template) == "" {
			return resource.Errorf("%s: bundles.identityTemplate is required", file)
		}
		matches := placeholder.FindAllStringSubmatch(template, -1)
		allowed := map[string]bool{"publisher": true, "target": true, "agent": true, "version": true}
		hasAgent, hasTarget := false, false
		for _, m := range matches {
			if !allowed[m[1]] {
				return resource.Errorf("%s: unknown identity template placeholder '{%s}'", file, m[1])
			}
			if m[1] == "agent" {
				hasAgent = true
			}
			if m[1] == "target" {
				hasTarget = true
			}
		}
		if !hasAgent || !hasTarget {
			return resource.Errorf("%s: bundles.identityTemplate must contain {agent} and {target}", file)
		}
		sample := ExpandIdentity(template, "example.com", "neutral", "agent", "1.0.0")
		if err := ValidateIdentity(sample, configuration.Bundles.IdentityType, "bundles.identityTemplate"); err != nil {
			return err
		}
		if err := validateProfile(&configuration.Bundles.Profile, "bundles"); err != nil {
			return err
		}
	}
	return nil
}

func validateProfile(profile *Profile, field string) error {
	if profile.TrustSchema != nil {
		if err := required(profile.TrustSchema.Identifier, field+".trustSchema.identifier"); err != nil {
			return err
		}
		if err := required(profile.TrustSchema.Version, field+".trustSchema.version"); err != nil {
			return err
		}
		if err := optionalURI(profile.TrustSchema.GovernanceURI, field+".trustSchema.governanceUri"); err != nil {
			return err
		}
		for _, method := range profile.TrustSchema.VerificationMethods {
			if strings.TrimSpace(method) == "" {
				return resource.Errorf("%s.trustSchema.verificationMethods cannot contain empty values", field)
			}
		}
	}
	for _, attestation := range profile.Attestations {
		if err := required(attestation.Type, field+".attestations.type"); err != nil {
			return err
		}
		if err := validateURI(attestation.URI, field+".attestations.uri", true); err != nil {
			return err
		}
		if err := optionalDigest(attestation.Digest, field+".attestations.digest"); err != nil {
			return err
		}
		if attestation.Size != nil && *attestation.Size < 0 {
			return resource.Errorf("%s.attestations.size cannot be negative", field)
		}
	}
	for _, link := range profile.Provenance {
		if err := required(link.Relation, field+".provenance.relation"); err != nil {
			return err
		}
		if err := required(link.SourceID, field+".provenance.sourceId"); err != nil {
			return err
		}
		if u, err := url.Parse(link.SourceID); err != nil || !u.IsAbs() {
			return resource.Errorf("%s.provenance.sourceId must be an absolute URI", field)
		}
		if err := optionalDigest(link.SourceDigest, field+".provenance.sourceDigest"); err != nil {
			return err
		}
		if err := optionalURI(link.RegistryURI, field+".provenance.registryUri"); err != nil {
			return err
		}
		if err := optionalURI(link.StatementURI, field+".provenance.statementUri"); err != nil {
			return err
		}
		if err := optionalURI(link.SignatureRef, field+".provenance.signatureRef"); err != nil {
			return err
		}
	}
	for _, key := range profile.Metadata.Keys() {
		if strings.TrimSpace(key) == "" {
			return resource.Errorf("%s.metadata keys cannot be empty", field)
		}
	}
	if profile.SignatureIntent != nil &&
		strings.TrimSpace(profile.SignatureIntent.Algorithm) == "" &&
		strings.TrimSpace(profile.SignatureIntent.KeyRef) == "" {
		return resource.Errorf("%s.signatureIntent requires algorithm or keyRef", field)
	}
	if profile.SignatureIntent != nil {
		if err := optionalURI(profile.SignatureIntent.KeyRef, field+".signatureIntent.keyRef"); err != nil {
			return err
		}
	}
	return nil
}

func required(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return resource.Errorf("%s is required", field)
	}
	return nil
}

func optionalURI(value, field string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return validateURI(value, field, false)
}

func optionalDigest(value, field string) error {
	if value == "" {
		return nil
	}
	if !digestPattern.MatchString(value) {
		return resource.Errorf("%s must be a lowercase SHA-256, SHA-384, or SHA-512 digest", field)
	}
	return nil
}

// --- YAML decoding -----------------------------------------------------------

func parseConfiguration(text string) (*Configuration, error) {
	decoder := yaml.NewDecoder(strings.NewReader(text))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	config := &Configuration{}
	err := decodeMapping(&node, map[string]func(*yaml.Node) error{
		"schemaVersion": intField(&config.SchemaVersion),
		"source": func(n *yaml.Node) error {
			profile := &SourceProfile{Profile: Profile{Metadata: NewMetadataMap()}}
			if err := decodeProfile(n, &profile.Profile, map[string]func(*yaml.Node) error{
				"identity": stringField(&profile.Identity),
			}); err != nil {
				return err
			}
			config.Source = profile
			return nil
		},
		"bundles": func(n *yaml.Node) error {
			profile := &BundleProfile{Profile: Profile{Metadata: NewMetadataMap()}}
			if err := decodeProfile(n, &profile.Profile, map[string]func(*yaml.Node) error{
				"identityTemplate": stringField(&profile.IdentityTemplate),
			}); err != nil {
				return err
			}
			config.Bundles = profile
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	return config, nil
}

func decodeProfile(node *yaml.Node, profile *Profile, extra map[string]func(*yaml.Node) error) error {
	fields := map[string]func(*yaml.Node) error{
		"identityType": stringField(&profile.IdentityType),
		"trustSchema": func(n *yaml.Node) error {
			schema := &Schema{}
			if err := decodeMapping(n, map[string]func(*yaml.Node) error{
				"identifier":          stringField(&schema.Identifier),
				"version":             stringField(&schema.Version),
				"governanceUri":       stringField(&schema.GovernanceURI),
				"verificationMethods": stringListField(&schema.VerificationMethods),
			}); err != nil {
				return err
			}
			profile.TrustSchema = schema
			return nil
		},
		"attestations": func(n *yaml.Node) error {
			if n.Kind != yaml.SequenceNode {
				return resource.Errorf("attestations must be a sequence")
			}
			for _, item := range n.Content {
				attestation := Attestation{}
				var size string
				hasSize := false
				if err := decodeMapping(item, map[string]func(*yaml.Node) error{
					"type":   stringField(&attestation.Type),
					"uri":    stringField(&attestation.URI),
					"digest": stringField(&attestation.Digest),
					"size": func(sn *yaml.Node) error {
						hasSize = true
						return scalarString(sn, &size)
					},
					"description": stringField(&attestation.Description),
				}); err != nil {
					return err
				}
				if hasSize {
					v, err := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
					if err != nil {
						return resource.Errorf("attestation size must be an integer")
					}
					attestation.Size = &v
				}
				profile.Attestations = append(profile.Attestations, attestation)
			}
			return nil
		},
		"provenance": func(n *yaml.Node) error {
			if n.Kind != yaml.SequenceNode {
				return resource.Errorf("provenance must be a sequence")
			}
			for _, item := range n.Content {
				link := ProvenanceLink{}
				if err := decodeMapping(item, map[string]func(*yaml.Node) error{
					"relation":     stringField(&link.Relation),
					"sourceId":     stringField(&link.SourceID),
					"sourceDigest": stringField(&link.SourceDigest),
					"registryUri":  stringField(&link.RegistryURI),
					"statementUri": stringField(&link.StatementURI),
					"signatureRef": stringField(&link.SignatureRef),
				}); err != nil {
					return err
				}
				profile.Provenance = append(profile.Provenance, link)
			}
			return nil
		},
		"metadata": func(n *yaml.Node) error {
			value, err := canonicalMetadataValue(n)
			if err != nil {
				return err
			}
			m, ok := value.(MetadataMap)
			if !ok {
				return resource.Errorf("metadata must be a mapping")
			}
			profile.Metadata = m
			return nil
		},
		"signatureIntent": func(n *yaml.Node) error {
			intent := &SignatureIntent{}
			if err := decodeMapping(n, map[string]func(*yaml.Node) error{
				"algorithm": stringField(&intent.Algorithm),
				"keyRef":    stringField(&intent.KeyRef),
				"required":  boolField(&intent.Required),
			}); err != nil {
				return err
			}
			profile.SignatureIntent = intent
			return nil
		},
	}
	for k, v := range extra {
		fields[k] = v
	}
	return decodeMapping(node, fields)
}

// canonicalMetadataValue mirrors the reference implementation: YAML scalars
// become strings, mappings become canonically ordered maps, sequences become
// lists, null stays null.
func canonicalMetadataValue(node *yaml.Node) (MetadataValue, error) {
	node = resolveAlias(node)
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return nil, nil
		}
		return node.Value, nil
	case yaml.MappingNode:
		m := NewMetadataMap()
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := resolveAlias(node.Content[i])
			if key.Kind != yaml.ScalarNode || !metadataKey.MatchString(key.Value) {
				return nil, resource.Errorf("Trust metadata keys must be ASCII identifiers matching [A-Za-z0-9][A-Za-z0-9._-]*")
			}
			value, err := canonicalMetadataValue(node.Content[i+1])
			if err != nil {
				return nil, err
			}
			m.Set(key.Value, value)
		}
		return m, nil
	case yaml.SequenceNode:
		list := MetadataList{}
		for _, item := range node.Content {
			value, err := canonicalMetadataValue(item)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		return list, nil
	default:
		return nil, resource.Errorf("unsupported metadata value")
	}
}

func resolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

func decodeMapping(node *yaml.Node, fields map[string]func(*yaml.Node) error) error {
	node = resolveAlias(node)
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		node = resolveAlias(node.Content[0])
	}
	if node.Kind != yaml.MappingNode {
		return resource.Errorf("expected a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := resolveAlias(node.Content[i])
		if key.Kind != yaml.ScalarNode {
			return resource.Errorf("mapping keys must be scalars")
		}
		decoder, known := fields[key.Value]
		if !known {
			return resource.Errorf("property '%s' not found", key.Value)
		}
		if err := decoder(resolveAlias(node.Content[i+1])); err != nil {
			return err
		}
	}
	return nil
}

func scalarString(node *yaml.Node, out *string) error {
	node = resolveAlias(node)
	if node.Kind != yaml.ScalarNode {
		return resource.Errorf("expected a scalar value")
	}
	if node.Tag == "!!null" {
		*out = ""
		return nil
	}
	*out = node.Value
	return nil
}

func stringField(target *string) func(*yaml.Node) error {
	return func(node *yaml.Node) error { return scalarString(node, target) }
}

func intField(target *int) func(*yaml.Node) error {
	return func(node *yaml.Node) error {
		var raw string
		if err := scalarString(node, &raw); err != nil {
			return err
		}
		v, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return resource.Errorf("expected an integer, got '%s'", raw)
		}
		*target = v
		return nil
	}
}

func boolField(target *bool) func(*yaml.Node) error {
	return func(node *yaml.Node) error {
		var raw string
		if err := scalarString(node, &raw); err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true":
			*target = true
		case "false":
			*target = false
		default:
			return resource.Errorf("expected true or false, got '%s'", raw)
		}
		return nil
	}
}

func stringListField(target *[]string) func(*yaml.Node) error {
	return func(node *yaml.Node) error {
		if node.Kind != yaml.SequenceNode {
			return resource.Errorf("expected a sequence")
		}
		items := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			var v string
			if err := scalarString(item, &v); err != nil {
				return err
			}
			items = append(items, v)
		}
		*target = items
		return nil
	}
}

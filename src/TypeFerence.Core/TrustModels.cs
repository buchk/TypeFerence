using System.Collections;
using System.Text.Json;
using System.Text.RegularExpressions;
using YamlDotNet.Core;
using YamlDotNet.Serialization;
using YamlDotNet.Serialization.NamingConventions;

namespace TypeFerence.Core;

public sealed class TrustConfiguration
{
    public int SchemaVersion { get; set; }
    public SourceTrustProfile? Source { get; set; }
    public BundleTrustProfile? Bundles { get; set; }
}

public class TrustProfile
{
    public string IdentityType { get; set; } = "";
    public TrustSchema? TrustSchema { get; set; }
    public List<TrustAttestation> Attestations { get; set; } = [];
    public List<TrustProvenanceLink> Provenance { get; set; } = [];
    public SortedDictionary<string, object?> Metadata { get; set; } = new(StringComparer.Ordinal);
    public SignatureIntent? SignatureIntent { get; set; }
}

public sealed class SourceTrustProfile : TrustProfile
{
    public string Identity { get; set; } = "";
}

public sealed class BundleTrustProfile : TrustProfile
{
    public string IdentityTemplate { get; set; } = "";
}

public sealed class TrustSchema
{
    public string Identifier { get; set; } = "";
    public string Version { get; set; } = "";
    public string? GovernanceUri { get; set; }
    public List<string> VerificationMethods { get; set; } = [];
}

public sealed class TrustAttestation
{
    public string Type { get; set; } = "";
    public string Uri { get; set; } = "";
    public string? Digest { get; set; }
    public long? Size { get; set; }
    public string? Description { get; set; }
}

public sealed class TrustProvenanceLink
{
    public string Relation { get; set; } = "";
    public string SourceId { get; set; } = "";
    public string? SourceDigest { get; set; }
    public string? RegistryUri { get; set; }
    public string? StatementUri { get; set; }
    public string? SignatureRef { get; set; }
}

public sealed class SignatureIntent
{
    public string? Algorithm { get; set; }
    public string? KeyRef { get; set; }
    public bool Required { get; set; }
}

internal sealed record LoadedTrustConfiguration(TrustConfiguration Configuration, string Path);

internal static partial class TrustConfigurationLoader
{
    internal const string DefaultFileName = "typeference.trust.yaml";
    internal const string TypeFerenceMetadataPrefix = "com.github.buchk.typeference";

    private static readonly IDeserializer Yaml = new DeserializerBuilder()
        .WithNamingConvention(CamelCaseNamingConvention.Instance)
        .Build();

    private static readonly HashSet<string> KnownIdentityTypes = new(StringComparer.Ordinal)
    {
        "did", "dns", "https", "spiffe"
    };

    internal static LoadedTrustConfiguration? Load(string sourceDirectory, string? configuredPath = null)
    {
        var root = Path.GetFullPath(sourceDirectory);
        var path = configuredPath is null
            ? Path.Combine(root, DefaultFileName)
            : Path.GetFullPath(configuredPath, Directory.GetCurrentDirectory());
        if (configuredPath is null && !File.Exists(path)) return null;
        EnsureBeneathRoot(root, path);
        if (!File.Exists(path)) throw new TypeFerenceException($"Trust configuration not found: {path}");

        TrustConfiguration configuration;
        try
        {
            configuration = Yaml.Deserialize<TrustConfiguration>(File.ReadAllText(path))
                ?? throw new TypeFerenceException($"Empty trust configuration: {path}");
        }
        catch (YamlException ex)
        {
            throw new TypeFerenceException($"{path}: invalid trust configuration: {ex.Message}");
        }

        NormalizeMetadata(configuration);
        Validate(configuration, path);
        return new(configuration, path);
    }

    internal static IReadOnlyDictionary<string, string> LoadSignatures(string path)
    {
        var full = Path.GetFullPath(path);
        if (!File.Exists(full)) throw new TypeFerenceException($"Trust signatures file not found: {full}");
        try
        {
            using var document = JsonDocument.Parse(File.ReadAllText(full));
            if (document.RootElement.ValueKind != JsonValueKind.Object)
                throw new TypeFerenceException("Trust signatures file must be a JSON object keyed by catalog identifier");
            var result = new SortedDictionary<string, string>(StringComparer.Ordinal);
            foreach (var property in document.RootElement.EnumerateObject())
            {
                if (property.Value.ValueKind != JsonValueKind.String)
                    throw new TypeFerenceException($"Trust signature for {property.Name} must be a string");
                var signature = property.Value.GetString()!;
                ValidateDetachedJws(signature, property.Name);
                if (!result.TryAdd(property.Name, signature))
                    throw new TypeFerenceException($"Duplicate trust signature identifier: {property.Name}");
            }
            return result;
        }
        catch (JsonException ex)
        {
            throw new TypeFerenceException($"{full}: invalid trust signatures JSON: {ex.Message}");
        }
    }

    internal static void ValidateIdentityForPublisher(string identity, string identityType, string publisherDomain, string field)
    {
        ValidateIdentity(identity, identityType, field);
        var uri = new Uri(identity);
        string? domain = null;
        if (identity.StartsWith("did:web:", StringComparison.Ordinal))
            domain = Uri.UnescapeDataString(identity[8..].Split(':', '/', '?', '#')[0]).Split(':')[0];
        else if (string.Equals(uri.Scheme, "dns", StringComparison.Ordinal))
            domain = identity[(identity.IndexOf(':') + 1)..].TrimStart('/').Split('/', '?', '#')[0];
        else if (!string.IsNullOrWhiteSpace(uri.Host)) domain = uri.IdnHost;
        if (string.IsNullOrWhiteSpace(domain))
            throw new TypeFerenceException($"{field} does not expose an authority or trust domain that can align with ARD publisher domain '{publisherDomain}'");
        if (!string.Equals(domain, publisherDomain, StringComparison.OrdinalIgnoreCase))
            throw new TypeFerenceException($"{field} domain '{domain}' does not align with ARD publisher domain '{publisherDomain}'");
    }

    internal static SortedDictionary<string, object?> CanonicalMetadata(IEnumerable<KeyValuePair<string, object?>> metadata) =>
        new(metadata.ToDictionary(x => CheckedMetadataKey(x.Key), x => CanonicalValue(x.Value), StringComparer.Ordinal), StringComparer.Ordinal);

    private static string CheckedMetadataKey(string key) => MetadataKey().IsMatch(key)
        ? key
        : throw new TypeFerenceException("Trust metadata keys must be ASCII identifiers matching [A-Za-z0-9][A-Za-z0-9._-]*");

    private static void Validate(TrustConfiguration configuration, string file)
    {
        if (configuration.SchemaVersion != 1) throw new TypeFerenceException($"{file}: schemaVersion must be 1");
        if (configuration.Source is null && configuration.Bundles is null)
            throw new TypeFerenceException($"{file}: at least one of source or bundles is required");
        if (configuration.Source is not null)
        {
            if (string.IsNullOrWhiteSpace(configuration.Source.Identity))
                throw new TypeFerenceException($"{file}: source.identity is required");
            ValidateIdentity(configuration.Source.Identity, configuration.Source.IdentityType, "source.identity");
            ValidateProfile(configuration.Source, "source");
        }
        if (configuration.Bundles is not null)
        {
            var template = configuration.Bundles.IdentityTemplate;
            if (string.IsNullOrWhiteSpace(template)) throw new TypeFerenceException($"{file}: bundles.identityTemplate is required");
            var placeholders = Placeholder().Matches(template).Select(x => x.Groups[1].Value).ToArray();
            var allowed = new HashSet<string>(["publisher", "target", "agent", "version"], StringComparer.Ordinal);
            var unknown = placeholders.FirstOrDefault(x => !allowed.Contains(x));
            if (unknown is not null) throw new TypeFerenceException($"{file}: unknown identity template placeholder '{{{unknown}}}'");
            if (!placeholders.Contains("agent", StringComparer.Ordinal) || !placeholders.Contains("target", StringComparer.Ordinal))
                throw new TypeFerenceException($"{file}: bundles.identityTemplate must contain {{agent}} and {{target}}");
            var sample = ExpandIdentity(template, "example.com", "neutral", "agent", "1.0.0");
            ValidateIdentity(sample, configuration.Bundles.IdentityType, "bundles.identityTemplate");
            ValidateProfile(configuration.Bundles, "bundles");
        }
    }

    internal static string ExpandIdentity(string template, string publisher, string target, string agent, string version) => template
        .Replace("{publisher}", publisher, StringComparison.Ordinal)
        .Replace("{target}", target, StringComparison.Ordinal)
        .Replace("{agent}", agent, StringComparison.Ordinal)
        .Replace("{version}", version, StringComparison.Ordinal);

    private static void ValidateProfile(TrustProfile profile, string field)
    {
        if (profile.IdentityType.Length > 0 && string.IsNullOrWhiteSpace(profile.IdentityType))
            throw new TypeFerenceException($"{field}.identityType cannot be whitespace");
        if (profile.TrustSchema is not null)
        {
            Required(profile.TrustSchema.Identifier, $"{field}.trustSchema.identifier");
            Required(profile.TrustSchema.Version, $"{field}.trustSchema.version");
            OptionalUri(profile.TrustSchema.GovernanceUri, $"{field}.trustSchema.governanceUri");
            if (profile.TrustSchema.VerificationMethods.Any(string.IsNullOrWhiteSpace))
                throw new TypeFerenceException($"{field}.trustSchema.verificationMethods cannot contain empty values");
        }
        foreach (var attestation in profile.Attestations)
        {
            Required(attestation.Type, $"{field}.attestations.type");
            ValidateUri(attestation.Uri, $"{field}.attestations.uri", allowData: true);
            OptionalDigest(attestation.Digest, $"{field}.attestations.digest");
            if (attestation.Size < 0) throw new TypeFerenceException($"{field}.attestations.size cannot be negative");
        }
        foreach (var provenance in profile.Provenance)
        {
            Required(provenance.Relation, $"{field}.provenance.relation");
            Required(provenance.SourceId, $"{field}.provenance.sourceId");
            if (!Uri.TryCreate(provenance.SourceId, UriKind.Absolute, out _))
                throw new TypeFerenceException($"{field}.provenance.sourceId must be an absolute URI");
            OptionalDigest(provenance.SourceDigest, $"{field}.provenance.sourceDigest");
            OptionalUri(provenance.RegistryUri, $"{field}.provenance.registryUri");
            OptionalUri(provenance.StatementUri, $"{field}.provenance.statementUri");
            OptionalUri(provenance.SignatureRef, $"{field}.provenance.signatureRef");
        }
        foreach (var key in profile.Metadata.Keys)
            if (string.IsNullOrWhiteSpace(key)) throw new TypeFerenceException($"{field}.metadata keys cannot be empty");
        if (profile.SignatureIntent is not null && string.IsNullOrWhiteSpace(profile.SignatureIntent.Algorithm) && string.IsNullOrWhiteSpace(profile.SignatureIntent.KeyRef))
            throw new TypeFerenceException($"{field}.signatureIntent requires algorithm or keyRef");
        OptionalUri(profile.SignatureIntent?.KeyRef, $"{field}.signatureIntent.keyRef");
    }

    private static void ValidateIdentity(string identity, string identityType, string field)
    {
        // Spec ("Canonical text and ordering"): identities must be ASCII so
        // domain alignment does not depend on an implementation's IDN
        // handling. Internationalized authorities must be pre-punycoded.
        if (identity.Any(c => c > 0x7F))
            throw new TypeFerenceException($"{field} must be ASCII; encode internationalized authorities as punycode");
        ValidateUri(identity, field);
        if (identity.StartsWith("did:", StringComparison.Ordinal) && !Did().IsMatch(identity))
            throw new TypeFerenceException($"{field} is not valid DID syntax");
        var expected = identity.StartsWith("did:", StringComparison.Ordinal) ? "did"
            : identity.StartsWith("spiffe://", StringComparison.Ordinal) ? "spiffe"
            : identity.StartsWith("https://", StringComparison.Ordinal) ? "https"
            : identity.StartsWith("dns:", StringComparison.Ordinal) ? "dns" : null;
        if (identityType.Length > 0 && KnownIdentityTypes.Contains(identityType) && expected is not null && !string.Equals(identityType, expected, StringComparison.Ordinal))
            throw new TypeFerenceException($"{field} scheme does not match identityType '{identityType}'");
    }

    private static void ValidateUri(string value, string field, bool allowData = false)
    {
        if (!Uri.TryCreate(value, UriKind.Absolute, out var uri)) throw new TypeFerenceException($"{field} must be an absolute URI");
        if (!allowData && string.Equals(uri.Scheme, "data", StringComparison.OrdinalIgnoreCase))
            throw new TypeFerenceException($"{field} cannot be a data URI");
        if (allowData && uri.Scheme is not ("https" or "data"))
            throw new TypeFerenceException($"{field} must use https or data");
    }

    private static void OptionalUri(string? value, string field)
    {
        if (!string.IsNullOrWhiteSpace(value)) ValidateUri(value, field);
    }

    private static void OptionalDigest(string? value, string field)
    {
        if (value is null) return;
        if (!Digest().IsMatch(value)) throw new TypeFerenceException($"{field} must be a lowercase SHA-256, SHA-384, or SHA-512 digest");
    }

    private static void ValidateDetachedJws(string value, string identifier)
    {
        var segments = value.Split('.');
        if (segments.Length != 3 || segments[0].Length == 0 || segments[1].Length != 0 || segments[2].Length == 0 ||
            !Base64Url().IsMatch(segments[0]) || !Base64Url().IsMatch(segments[2]))
            throw new TypeFerenceException($"Trust signature for {identifier} must be compact detached JWS (protected..signature)");
    }

    private static void Required(string value, string field)
    {
        if (string.IsNullOrWhiteSpace(value)) throw new TypeFerenceException($"{field} is required");
    }

    private static void EnsureBeneathRoot(string root, string path)
    {
        if (!path.StartsWith(root + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase))
            throw new TypeFerenceException($"Trust configuration must be beneath source root: {path}");
    }

    private static void NormalizeMetadata(TrustConfiguration configuration)
    {
        if (configuration.Source is not null) configuration.Source.Metadata = CanonicalMetadata(configuration.Source.Metadata);
        if (configuration.Bundles is not null) configuration.Bundles.Metadata = CanonicalMetadata(configuration.Bundles.Metadata);
    }

    private static object? CanonicalValue(object? value)
    {
        if (value is null || value is string || value is bool || value is byte || value is sbyte || value is short || value is ushort ||
            value is int || value is uint || value is long || value is ulong || value is float || value is double || value is decimal)
            return value;
        if (value is IDictionary dictionary)
        {
            var result = new SortedDictionary<string, object?>(StringComparer.Ordinal);
            foreach (DictionaryEntry entry in dictionary)
            {
                if (entry.Key is not string key || !MetadataKey().IsMatch(key)) throw new TypeFerenceException("Trust metadata keys must be ASCII identifiers matching [A-Za-z0-9][A-Za-z0-9._-]*");
                result[key] = CanonicalValue(entry.Value);
            }
            return result;
        }
        if (value is IEnumerable sequence) return sequence.Cast<object?>().Select(CanonicalValue).ToArray();
        return value.ToString();
    }

    [GeneratedRegex("\\{([^{}]+)\\}", RegexOptions.CultureInvariant)]
    private static partial Regex Placeholder();
    [GeneratedRegex("^did:[a-z0-9]+:[A-Za-z0-9._:%-]+(?::[A-Za-z0-9._:%-]+)*(?:/[^?#]*)?(?:\\?[^#]*)?(?:#.*)?$", RegexOptions.CultureInvariant)]
    private static partial Regex Did();
    [GeneratedRegex("^(?:sha256:[0-9a-f]{64}|sha384:[0-9a-f]{96}|sha512:[0-9a-f]{128})$", RegexOptions.CultureInvariant)]
    private static partial Regex Digest();
    [GeneratedRegex("^[A-Za-z0-9_-]+$", RegexOptions.CultureInvariant)]
    private static partial Regex Base64Url();
    [GeneratedRegex("^[A-Za-z0-9][A-Za-z0-9._-]*$", RegexOptions.CultureInvariant)]
    private static partial Regex MetadataKey();
}

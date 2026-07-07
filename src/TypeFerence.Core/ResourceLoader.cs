using System.Text.Json;
using System.Text.RegularExpressions;
using YamlDotNet.Core;
using YamlDotNet.Serialization;
using YamlDotNet.Serialization.NamingConventions;

namespace TypeFerence.Core;

public sealed class ResourceLoader
{
    private readonly IDeserializer _yaml = new DeserializerBuilder()
        .WithNamingConvention(CamelCaseNamingConvention.Instance)
        .Build();

    private static readonly Regex ResourceId = new(
        "^[a-z0-9][a-z0-9.-]*(?:/[a-z0-9][a-z0-9.-]*)+@[0-9]+\\.[0-9]+\\.[0-9]+(?:-[0-9A-Za-z.-]+)?$",
        RegexOptions.CultureInvariant);

    public IReadOnlyDictionary<string, ResourceDocument> Load(string sourceDirectory, string? trustConfigurationPath = null)
    {
        var root = Path.GetFullPath(sourceDirectory);
        if (!Directory.Exists(root)) throw new TypeFerenceException($"Source directory not found: {root}");
        var result = new SortedDictionary<string, ResourceDocument>(StringComparer.Ordinal);
        var excludedTrustFiles = new HashSet<string>(StringComparer.OrdinalIgnoreCase)
        {
            Path.Combine(root, TrustConfigurationLoader.DefaultFileName)
        };
        if (trustConfigurationPath is not null) excludedTrustFiles.Add(Path.GetFullPath(trustConfigurationPath));
        foreach (var file in Directory.EnumerateFiles(root, "*.yaml", SearchOption.AllDirectories)
                     .Where(x => !excludedTrustFiles.Contains(Path.GetFullPath(x)))
                     .Order(StringComparer.Ordinal))
        {
            ResourceDocument resource;
            try
            {
                resource = _yaml.Deserialize<ResourceDocument>(File.ReadAllText(file))
                    ?? throw new TypeFerenceException($"Empty resource: {file}");
            }
            catch (YamlException ex)
            {
                throw new TypeFerenceException($"{file}: invalid YAML resource: {ex.Message}");
            }
            ValidateShape(resource, file, root);
            if (!result.TryAdd(resource.Id, resource)) throw new TypeFerenceException($"Duplicate resource id: {resource.Id}");
        }
        if (result.Count == 0) throw new TypeFerenceException($"No YAML resources found under {root}");
        return result;
    }

    private static void ValidateShape(ResourceDocument resource, string file, string root)
    {
        if (resource.SchemaVersion != 3) throw new TypeFerenceException($"{file}: schemaVersion must be 3");
        if (resource.Kind is not ("agent" or "profile" or "interface" or "capability" or "skill")) throw new TypeFerenceException($"{file}: unknown kind '{resource.Kind}'");
        if (!ResourceId.IsMatch(resource.Id))
            throw new TypeFerenceException($"{file}: id must use lowercase namespace/name@semantic-version");
        if (resource.Kind is "capability" or "skill" && resource.Embeds.Count != 0) throw new TypeFerenceException($"{file}: {resource.Kind}s cannot embed resources");
        if (resource.Kind == "skill" && string.IsNullOrWhiteSpace(resource.Binds)) throw new TypeFerenceException($"{file}: skills must bind a capability");
        if (resource.Kind == "skill" && !ResourceId.IsMatch(resource.Binds)) throw new TypeFerenceException($"{file}: binds must reference a capability id");
        if (resource.Kind != "skill" && !string.IsNullOrWhiteSpace(resource.Binds)) throw new TypeFerenceException($"{file}: only skills can bind capabilities");
        foreach (var relative in resource.ContextFiles.Concat(resource.Slots.Values))
        {
            var full = Path.GetFullPath(Path.Combine(root, relative.Replace('/', Path.DirectorySeparatorChar)));
            if (!full.StartsWith(root + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase))
                throw new TypeFerenceException($"{file}: path escapes source root: {relative}");
            if (!File.Exists(full)) throw new TypeFerenceException($"{file}: referenced file does not exist: {relative}");
        }
        ValidateJson(resource.InputSchema, file, "inputSchema");
        ValidateJson(resource.OutputSchema, file, "outputSchema");
    }

    private static void ValidateJson(string value, string file, string field)
    {
        try { using var _ = JsonDocument.Parse(value); }
        catch (JsonException ex) { throw new TypeFerenceException($"{file}: invalid {field}: {ex.Message}"); }
    }
}

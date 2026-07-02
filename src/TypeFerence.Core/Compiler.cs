using System.Security.Cryptography;
using System.Text;
using System.Text.Json;

namespace TypeFerence.Core;

public enum CompilationTarget { Neutral, Codex, Copilot, Cursor }

public sealed record ArdPublicationOptions(string PublisherDomain);

public sealed class TypeFerenceCompiler
{
    private static readonly JsonSerializerOptions JsonOptions = new() { WriteIndented = true, PropertyNamingPolicy = JsonNamingPolicy.CamelCase };

    public IReadOnlyList<ResolvedAgent> Validate(string source)
    {
        var resources = new ResourceLoader().Load(source);
        return new TypeResolver(resources).ResolveAll();
    }

    public IReadOnlyList<string> Build(
        string source,
        string output,
        IEnumerable<CompilationTarget> targets,
        ArdPublicationOptions? ardPublication = null)
    {
        var agents = Validate(source).Where(x => !x.Abstract).OrderBy(x => x.Id, StringComparer.Ordinal).ToArray();
        var requestedTargets = targets.Distinct().Order().ToArray();
        if (requestedTargets.Length == 0) throw new TypeFerenceException("At least one compilation target is required");
        var root = Path.GetFullPath(output);
        Directory.CreateDirectory(root);
        var written = new List<string>();
        foreach (var target in requestedTargets)
        {
            var targetRoot = Path.Combine(root, target.ToString().ToLowerInvariant());
            if (Directory.Exists(targetRoot)) Directory.Delete(targetRoot, true);
            Directory.CreateDirectory(targetRoot);
            foreach (var agent in agents) WriteTarget(target, targetRoot, agent, written);
        }
        if (ardPublication is not null)
        {
            var ardRoot = Path.Combine(root, "ard");
            if (Directory.Exists(ardRoot)) Directory.Delete(ardRoot, true);
            Directory.CreateDirectory(ardRoot);
            WriteArdCatalog(ardRoot, source, root, agents, requestedTargets, ardPublication.PublisherDomain, written);
        }
        return written.Order(StringComparer.Ordinal).ToArray();
    }

    public static InvocationPackage Invoke(ResolvedAgent agent, ResolvedSkill skill, JsonElement arguments)
    {
        ValidateArguments(skill.InputSchema, arguments);
        return new(
            agent.Id,
            skill.ImplementationId,
            skill.DispatchName,
            JsonSerializer.Deserialize<object>(arguments.GetRawText()) ?? new { },
            skill.Instructions,
            skill.ContextFiles,
            new SortedDictionary<string, string>(StringComparer.Ordinal)
            {
                ["codex"] = ".agents/skills",
                ["copilot"] = ".github/agents",
                ["cursor"] = ".cursor/rules"
            },
            skill.Provenance);
    }

    public static IReadOnlyList<ResolvedAgent> LoadCompiled(string directory)
    {
        var options = new JsonSerializerOptions { PropertyNameCaseInsensitive = true };
        var agents = Directory.EnumerateFiles(directory, "bundle.json", SearchOption.AllDirectories)
            .Order(StringComparer.Ordinal)
            .Select(x => JsonSerializer.Deserialize<ResolvedAgent>(File.ReadAllText(x), options)
                ?? throw new TypeFerenceException($"Invalid compiled bundle: {x}"))
            .ToArray();
        if (agents.Length == 0) throw new TypeFerenceException($"No compiled bundle.json files found under {directory}");
        return agents;
    }

    public static string HashDirectory(string directory)
    {
        using var sha = SHA256.Create();
        var payload = new StringBuilder();
        foreach (var file in Directory.EnumerateFiles(directory, "*", SearchOption.AllDirectories).Order(StringComparer.Ordinal))
            payload.Append(Path.GetRelativePath(directory, file).Replace('\\', '/')).Append('\0').Append(File.ReadAllText(file).Replace("\r\n", "\n")).Append('\0');
        return Convert.ToHexString(sha.ComputeHash(Encoding.UTF8.GetBytes(payload.ToString()))).ToLowerInvariant();
    }

    private static void WriteTarget(CompilationTarget target, string root, ResolvedAgent agent, List<string> written)
    {
        var slug = Slug(agent.Id);
        switch (target)
        {
            case CompilationTarget.Neutral:
                Write(Path.Combine(root, slug, "AGENTS.md"), RenderInstructions(agent), written);
                Write(Path.Combine(root, slug, "bundle.json"), JsonSerializer.Serialize(agent, JsonOptions) + "\n", written);
                Write(Path.Combine(root, slug, "provenance.json"), JsonSerializer.Serialize(agent.Provenance, JsonOptions) + "\n", written);
                foreach (var skill in agent.Skills)
                    Write(Path.Combine(root, slug, "skills", SkillSlug(skill), "SKILL.md"), RenderSkill(skill), written);
                break;
            case CompilationTarget.Codex:
                Write(Path.Combine(root, slug, "AGENTS.md"), RenderInstructions(agent), written);
                foreach (var skill in agent.Skills)
                    Write(Path.Combine(root, slug, ".agents", "skills", SkillSlug(skill), "SKILL.md"), RenderSkill(skill), written);
                Write(Path.Combine(root, slug, ".typeference", "bundle.json"), JsonSerializer.Serialize(agent, JsonOptions) + "\n", written);
                Write(Path.Combine(root, slug, ".codex", "config.toml"), "[mcp_servers.typeference]\ncommand = \"typeference\"\nargs = [\"serve\", \".typeference\"]\n", written);
                break;
            case CompilationTarget.Copilot:
                Write(Path.Combine(root, slug, ".github", "copilot-instructions.md"), RenderInstructions(agent), written);
                Write(Path.Combine(root, slug, ".github", "agents", slug + ".agent.md"), $"---\nname: {slug}\ndescription: {EscapeYaml(agent.Description)}\n---\n\n{RenderInstructions(agent)}", written);
                Write(Path.Combine(root, slug, ".typeference", "bundle.json"), JsonSerializer.Serialize(agent, JsonOptions) + "\n", written);
                break;
            case CompilationTarget.Cursor:
                Write(Path.Combine(root, slug, "AGENTS.md"), RenderInstructions(agent), written);
                Write(Path.Combine(root, slug, ".cursor", "rules", slug + ".mdc"), $"---\ndescription: {EscapeYaml(agent.Description)}\nglobs:\nalwaysApply: true\n---\n\n{RenderInstructions(agent)}", written);
                Write(Path.Combine(root, slug, ".typeference", "bundle.json"), JsonSerializer.Serialize(agent, JsonOptions) + "\n", written);
                break;
        }
    }

    private static void WriteArdCatalog(
        string ardRoot,
        string source,
        string outputRoot,
        IReadOnlyList<ResolvedAgent> agents,
        IReadOnlyList<CompilationTarget> targets,
        string publisherDomain,
        List<string> written)
    {
        if (!System.Text.RegularExpressions.Regex.IsMatch(
                publisherDomain,
                "^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\\.)+[a-z]{2,63}$",
                System.Text.RegularExpressions.RegexOptions.CultureInvariant))
            throw new TypeFerenceException($"Invalid ARD publisher domain: {publisherDomain}");

        var sourceName = UrnSegment(Path.GetFileName(Path.GetFullPath(source).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar)));
        var sourceIdentifier = $"urn:air:{publisherDomain}:typeference:source:{sourceName}";
        var sourceDigest = "sha256:" + HashDirectory(source);
        var entries = new List<object>
        {
            new
            {
                identifier = sourceIdentifier,
                displayName = $"TypeFerence source package: {sourceName}",
                type = "application/vnd.typeference.source-package+json",
                description = "Canonical typed source package for validation, audit, and reproducible compilation.",
                version = "1.0.0",
                data = new
                {
                    schemaVersion = 1,
                    digest = sourceDigest,
                    files = PackageFiles(source)
                },
                metadata = new SortedDictionary<string, object>(StringComparer.Ordinal)
                {
                    ["generatedBy"] = "TypeFerence",
                    ["role"] = "canonical-source"
                }
            }
        };

        foreach (var target in targets)
            foreach (var agent in agents)
            {
                var targetName = target.ToString().ToLowerInvariant();
                var agentRoot = Path.Combine(outputRoot, targetName, Slug(agent.Id));
                entries.Add(new
                {
                    identifier = $"urn:air:{publisherDomain}:typeference:{targetName}:{Slug(agent.Id)}",
                    displayName = $"{agent.DisplayName} ({targetName})",
                    type = "application/vnd.typeference.target-bundle+json",
                    description = $"Precompiled {targetName} artifact bundle. {agent.Description}",
                    capabilities = agent.Skills.Select(x => x.DispatchName).Order(StringComparer.Ordinal).ToArray(),
                    version = agent.Id.Split('@').Last(),
                    data = new
                    {
                        schemaVersion = 1,
                        target = targetName,
                        agentId = agent.Id,
                        files = PackageFiles(agentRoot)
                    },
                    metadata = new SortedDictionary<string, object>(StringComparer.Ordinal)
                    {
                        ["generatedBy"] = "TypeFerence",
                        ["sourceIdentifier"] = sourceIdentifier,
                        ["sourceDigest"] = sourceDigest,
                        ["target"] = targetName
                    },
                    trustManifest = new
                    {
                        identity = $"https://{publisherDomain}",
                        identityType = "https",
                        provenance = new[]
                        {
                        new { relation = "derivedFrom", sourceId = sourceIdentifier, sourceDigest }
                    }
                    }
                });
            }
        var catalog = new
        {
            specVersion = "1.0",
            host = new { displayName = publisherDomain, identifier = publisherDomain },
            entries = entries.ToArray()
        };
        Write(Path.Combine(ardRoot, "ai-catalog.json"), JsonSerializer.Serialize(catalog, JsonOptions) + "\n", written);
    }

    private static object[] PackageFiles(string root) => Directory.EnumerateFiles(root, "*", SearchOption.AllDirectories)
        .Order(StringComparer.Ordinal)
        .Select(file => (object)new
        {
            path = Path.GetRelativePath(root, file).Replace('\\', '/'),
            mediaType = MediaType(file),
            content = File.ReadAllText(file).Replace("\r\n", "\n")
        })
        .ToArray();

    private static string MediaType(string path) => Path.GetExtension(path).ToLowerInvariant() switch
    {
        ".json" => "application/json",
        ".toml" => "application/toml",
        ".yaml" or ".yml" => "application/yaml",
        ".md" or ".mdc" => "text/markdown",
        _ => "text/plain"
    };

    private static string UrnSegment(string value)
    {
        var segment = System.Text.RegularExpressions.Regex.Replace(value.ToLowerInvariant(), "[^a-z0-9-]+", "-").Trim('-');
        return string.IsNullOrEmpty(segment) ? "package" : segment;
    }

    private static string RenderInstructions(ResolvedAgent agent)
    {
        var b = new StringBuilder($"# {agent.DisplayName}\n\n{agent.Description}\n\n");
        if (agent.WorkingNorms.Count > 0)
        {
            b.Append("## Working norms\n\n");
            foreach (var norm in agent.WorkingNorms) b.Append("- ").Append(norm).Append('\n');
            b.Append('\n');
        }
        if (agent.Slots.Count > 0)
        {
            b.Append("## Context slots\n\n");
            foreach (var slot in agent.Slots) b.Append("- `").Append(slot.Key).Append("`: `").Append(slot.Value).Append("`\n");
            b.Append('\n');
        }
        b.Append("## Available skills\n\n");
        foreach (var skill in agent.Skills) b.Append("- `").Append(skill.DispatchName).Append("`: ").Append(skill.Description).Append('\n');
        return b.Append('\n').ToString();
    }

    private static string RenderSkill(ResolvedSkill skill) => $"---\nname: {SkillSlug(skill)}\ndescription: {EscapeYaml(skill.Description)}\n---\n\n{skill.Instructions.Trim()}\n\n## Context loaded on invocation\n\n{string.Join("\n", skill.ContextFiles.Select(x => $"- `{x}`"))}\n";
    private static string SkillSlug(ResolvedSkill skill) => skill.ContractId.Split('/').Last().Split('@')[0];
    private static string Slug(string id) => id.Split('/').Last().Split('@')[0];
    private static string EscapeYaml(string value) => "\"" + value.Replace("\\", "\\\\").Replace("\"", "\\\"") + "\"";

    private static void Write(string path, string content, List<string> written)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, content.Replace("\r\n", "\n"), new UTF8Encoding(false));
        written.Add(path);
    }

    private static void ValidateArguments(string schemaJson, JsonElement arguments)
    {
        if (arguments.ValueKind != JsonValueKind.Object) throw new TypeFerenceException("Skill arguments must be a JSON object");
        using var schema = JsonDocument.Parse(schemaJson);
        var root = schema.RootElement;
        var names = arguments.EnumerateObject().Select(x => x.Name).ToHashSet(StringComparer.Ordinal);
        if (root.TryGetProperty("required", out var required))
            foreach (var item in required.EnumerateArray())
                if (!names.Contains(item.GetString()!)) throw new TypeFerenceException($"Missing required skill argument: {item.GetString()}");
        if (root.TryGetProperty("additionalProperties", out var additional) && additional.ValueKind == JsonValueKind.False &&
            root.TryGetProperty("properties", out var properties))
        {
            var allowed = properties.EnumerateObject().Select(x => x.Name).ToHashSet(StringComparer.Ordinal);
            var unknown = names.FirstOrDefault(x => !allowed.Contains(x));
            if (unknown is not null) throw new TypeFerenceException($"Unknown skill argument: {unknown}");
        }
    }
}

public sealed record DiffResult(bool Different, IReadOnlyList<string> Added, IReadOnlyList<string> Removed, IReadOnlyList<string> Changed)
{
    public static DiffResult Compare(string expected, string actual)
    {
        var left = Files(expected);
        var right = Files(actual);
        var added = right.Keys.Except(left.Keys, StringComparer.Ordinal).Order(StringComparer.Ordinal).ToArray();
        var removed = left.Keys.Except(right.Keys, StringComparer.Ordinal).Order(StringComparer.Ordinal).ToArray();
        var changed = left.Keys.Intersect(right.Keys, StringComparer.Ordinal).Where(x => left[x] != right[x]).Order(StringComparer.Ordinal).ToArray();
        return new(added.Length + removed.Length + changed.Length > 0, added, removed, changed);
    }

    private static SortedDictionary<string, string> Files(string root) => Directory.Exists(root)
        ? new(Directory.EnumerateFiles(root, "*", SearchOption.AllDirectories).ToDictionary(x => Path.GetRelativePath(root, x).Replace('\\', '/'), File.ReadAllText, StringComparer.Ordinal), StringComparer.Ordinal)
        : new(StringComparer.Ordinal);
}

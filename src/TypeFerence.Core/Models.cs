using System.Text.Json.Serialization;

namespace TypeFerence.Core;

public sealed class ResourceDocument
{
    public int SchemaVersion { get; set; }
    public string Kind { get; set; } = "";
    public string Id { get; set; } = "";
    public string DisplayName { get; set; } = "";
    public string Description { get; set; } = "";
    public string Binds { get; set; } = "";
    public bool Emit { get; set; } = true;
    public List<string> Embeds { get; set; } = [];
    public List<string> RequiresSlots { get; set; } = [];
    public List<string> RequiresCapabilities { get; set; } = [];
    public SortedDictionary<string, string> Slots { get; set; } = new(StringComparer.Ordinal);
    public List<string> WorkingNorms { get; set; } = [];
    public List<string> ContextFiles { get; set; } = [];
    public List<SkillBinding> Skills { get; set; } = [];
    public string Instructions { get; set; } = "";
    public string InputSchema { get; set; } = "{\"type\":\"object\",\"additionalProperties\":false}";
    public string OutputSchema { get; set; } = "{\"type\":\"object\"}";
}

public sealed class SkillBinding
{
    public string Ref { get; set; } = "";
    public string? Capability { get; set; }
}

public sealed record ProvenanceEntry(string Field, string Source);

public sealed class ResolvedSkill
{
    public string DispatchName { get; init; } = "";
    public string CapabilityId { get; init; } = "";
    public string ImplementationId { get; init; } = "";
    public string Description { get; init; } = "";
    public string Instructions { get; init; } = "";
    public string InputSchema { get; init; } = "";
    public string OutputSchema { get; init; } = "";
    public IReadOnlyList<string> ContextFiles { get; init; } = [];
    public IReadOnlyList<ProvenanceEntry> Provenance { get; init; } = [];
}

public sealed class ResolvedAgent
{
    public string Id { get; init; } = "";
    public string DisplayName { get; init; } = "";
    public string Description { get; init; } = "";
    public bool Emit { get; init; } = true;
    public IReadOnlyList<string> Embeds { get; init; } = [];
    public IReadOnlyList<string> Satisfies { get; init; } = [];
    public IReadOnlyDictionary<string, string> Slots { get; init; } = new SortedDictionary<string, string>();
    public IReadOnlyList<string> WorkingNorms { get; init; } = [];
    public IReadOnlyList<string> ContextFiles { get; init; } = [];
    public IReadOnlyList<ResolvedSkill> Skills { get; init; } = [];
    public IReadOnlyList<ProvenanceEntry> Provenance { get; init; } = [];
}

public sealed record InvocationPackage(
    string AgentId,
    string SkillId,
    string DispatchName,
    object Arguments,
    string Instructions,
    IReadOnlyList<string> ContextReferences,
    IReadOnlyDictionary<string, string> TargetHints,
    IReadOnlyList<ProvenanceEntry> Provenance);

public sealed class TypeFerenceException(string message) : Exception(message);

using System.Text.Json;

namespace TypeFerence.Core;

public sealed class TypeResolver(IReadOnlyDictionary<string, ResourceDocument> resources)
{
    private readonly Dictionary<string, ResolvedAgent> _cache = new(StringComparer.Ordinal);

    public IReadOnlyList<ResolvedAgent> ResolveAll() => resources.Values
        .Where(x => x.Kind == "agent")
        .OrderBy(x => x.Id, StringComparer.Ordinal)
        .Select(x => Resolve(x.Id))
        .ToArray();

    public ResolvedAgent Resolve(string id) => Resolve(id, new HashSet<string>(StringComparer.Ordinal));

    private ResolvedAgent Resolve(string id, HashSet<string> visiting)
    {
        if (_cache.TryGetValue(id, out var cached)) return cached;
        if (!resources.TryGetValue(id, out var current) || current.Kind != "agent") throw new TypeFerenceException($"Agent not found: {id}");
        if (!visiting.Add(id)) throw new TypeFerenceException($"Inheritance cycle detected at {id}");
        if (id == "system/object@1.0.0") ValidateSystemObject(current);
        if (id != "system/object@1.0.0" && current.Extends is null) throw new TypeFerenceException($"Agent {id} must extend another agent");

        var parent = current.Extends is null ? null : Resolve(current.Extends, visiting);
        if (parent is not null && parent.Id == "system/object@1.0.0" && Namespace(id) == "system")
            throw new TypeFerenceException("Only enterprise-owned agents may directly extend system/object");
        if (parent is not null && parent.Id == "system/object@1.0.0" && !current.Abstract)
            throw new TypeFerenceException($"Enterprise root {id} must be abstract");

        var lineage = parent?.Lineage.ToList() ?? [];
        lineage.Add(id);
        var slots = new SortedDictionary<string, string>(StringComparer.Ordinal);
        if (parent is not null)
            foreach (var pair in parent.Slots) slots[pair.Key] = pair.Value;
        foreach (var pair in current.Slots) slots[pair.Key] = NormalizePath(pair.Value);
        var norms = Distinct((parent?.WorkingNorms ?? []).Concat(current.WorkingNorms));
        var contexts = Distinct((parent?.ContextFiles ?? []).Concat(current.ContextFiles).Select(NormalizePath));
        var interfaceIds = Distinct((parent?.Interfaces ?? []).Concat(current.Implements));
        var provenance = parent?.Provenance.ToList() ?? [];
        if (!string.IsNullOrWhiteSpace(current.DisplayName)) provenance.Add(new("displayName", id));
        if (!string.IsNullOrWhiteSpace(current.Description)) provenance.Add(new("description", id));
        provenance.AddRange(current.Slots.Keys.Select(x => new ProvenanceEntry($"slots.{x}", id)));
        provenance.AddRange(current.WorkingNorms.Select(_ => new ProvenanceEntry("workingNorms", id)));
        provenance.AddRange(current.ContextFiles.Select(_ => new ProvenanceEntry("contextFiles", id)));
        provenance.AddRange(current.Implements.Select(x => new ProvenanceEntry($"interfaces.{x}", id)));

        var skills = parent?.Skills.ToDictionary(x => x.ContractId, StringComparer.Ordinal)
            ?? new Dictionary<string, ResolvedSkill>(StringComparer.Ordinal);
        foreach (var binding in current.Skills)
        {
            var implementation = Require(binding.Ref, "skill");
            var contract = binding.Overrides ?? binding.Ref;
            if (binding.Overrides is not null)
            {
                if (!skills.TryGetValue(contract, out var inherited)) throw new TypeFerenceException($"{id}: cannot override missing skill {contract}");
                EnsureSameContract(inherited, implementation, id);
            }
            skills[contract] = new ResolvedSkill
            {
                DispatchName = $"{UnversionedLeaf(id)}.{UnversionedLeaf(contract)}",
                ContractId = contract,
                ImplementationId = implementation.Id,
                Description = implementation.Description,
                Instructions = implementation.Instructions,
                InputSchema = CanonicalJson(implementation.InputSchema),
                OutputSchema = CanonicalJson(implementation.OutputSchema),
                ContextFiles = Distinct(contexts.Concat(implementation.ContextFiles.Select(NormalizePath))),
                Provenance = [new("skill.contract", contract), new("skill.implementation", implementation.Id)]
            };
        }

        foreach (var interfaceId in interfaceIds)
        {
            var contract = Require(interfaceId, "interface");
            foreach (var slot in contract.RequiresSlots)
                if (!slots.ContainsKey(slot)) throw new TypeFerenceException($"{id}: interface {interfaceId} requires slot '{slot}'");
            foreach (var skill in contract.RequiresSkills)
                if (!skills.ContainsKey(skill)) throw new TypeFerenceException($"{id}: interface {interfaceId} requires skill '{skill}'");
        }

        var resolved = new ResolvedAgent
        {
            Id = id,
            DisplayName = string.IsNullOrWhiteSpace(current.DisplayName) ? parent?.DisplayName ?? id : current.DisplayName,
            Description = string.IsNullOrWhiteSpace(current.Description) ? parent?.Description ?? "" : current.Description,
            Abstract = current.Abstract,
            Lineage = lineage,
            Interfaces = interfaceIds,
            Slots = slots,
            WorkingNorms = norms,
            ContextFiles = contexts,
            Skills = skills.Values.OrderBy(x => x.ContractId, StringComparer.Ordinal).Select(x => x.WithDispatch(id)).ToArray(),
            Provenance = provenance
        };
        visiting.Remove(id);
        return _cache[id] = resolved;
    }

    private ResourceDocument Require(string id, string kind)
    {
        if (!resources.TryGetValue(id, out var resource) || resource.Kind != kind) throw new TypeFerenceException($"Missing {kind}: {id}");
        return resource;
    }

    private static void EnsureSameContract(ResolvedSkill inherited, ResourceDocument replacement, string agent)
    {
        if (CanonicalJson(inherited.InputSchema) != CanonicalJson(replacement.InputSchema) ||
            CanonicalJson(inherited.OutputSchema) != CanonicalJson(replacement.OutputSchema))
            throw new TypeFerenceException($"{agent}: override {replacement.Id} changes the public contract of {inherited.ContractId}");
    }

    private static void ValidateSystemObject(ResourceDocument value)
    {
        if (!value.Abstract || value.Extends is not null || value.Implements.Count != 0 || value.Skills.Count != 0 ||
            value.Slots.Count != 0 || value.WorkingNorms.Count != 0 || value.ContextFiles.Count != 0 || !string.IsNullOrWhiteSpace(value.Instructions))
            throw new TypeFerenceException("system/object must be abstract and behavior-free");
    }

    private static string Namespace(string id) => id.Split('/')[0];
    private static string UnversionedLeaf(string id) => id.Split('/').Last().Split('@')[0];
    private static string NormalizePath(string value) => value.Replace('\\', '/').TrimStart('/');
    private static IReadOnlyList<string> Distinct(IEnumerable<string> values) => values.Distinct(StringComparer.Ordinal).ToArray();
    private static string CanonicalJson(string json) => JsonSerializer.Serialize(JsonDocument.Parse(json).RootElement);
}

internal static class ResolvedSkillExtensions
{
    internal static ResolvedSkill WithDispatch(this ResolvedSkill skill, string agentId) => new()
    {
        DispatchName = $"{agentId.Split('/').Last().Split('@')[0]}.{skill.ContractId.Split('/').Last().Split('@')[0]}",
        ContractId = skill.ContractId,
        ImplementationId = skill.ImplementationId,
        Description = skill.Description,
        Instructions = skill.Instructions,
        InputSchema = skill.InputSchema,
        OutputSchema = skill.OutputSchema,
        ContextFiles = skill.ContextFiles,
        Provenance = skill.Provenance
    };
}

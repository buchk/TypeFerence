using System.Text.Json;

namespace TypeFerence.Core;

public sealed class TypeResolver(IReadOnlyDictionary<string, ResourceDocument> resources)
{
    private readonly Dictionary<string, ResolvedAgent> _agentCache = new(StringComparer.Ordinal);
    private readonly Dictionary<string, InterfaceContract> _interfaceCache = new(StringComparer.Ordinal);
    private readonly Dictionary<string, IReadOnlyDictionary<string, int>> _slotDepths = new(StringComparer.Ordinal);
    private readonly Dictionary<string, IReadOnlyDictionary<string, int>> _skillDepths = new(StringComparer.Ordinal);

    public IReadOnlyList<ResolvedAgent> ResolveAll()
    {
        foreach (var item in resources.Values.Where(x => x.Kind == "interface").OrderBy(x => x.Id, StringComparer.Ordinal))
            ResolveInterface(item.Id, new HashSet<string>(StringComparer.Ordinal));
        return resources.Values
            .Where(x => x.Kind == "agent")
            .OrderBy(x => x.Id, StringComparer.Ordinal)
            .Select(x => Resolve(x.Id))
            .ToArray();
    }

    public ResolvedAgent Resolve(string id) => ResolveAgent(id, new HashSet<string>(StringComparer.Ordinal));

    private ResolvedAgent ResolveAgent(string id, HashSet<string> visiting)
    {
        if (_agentCache.TryGetValue(id, out var cached)) return cached;
        var current = Require(id, "agent");
        if (!visiting.Add(id)) throw new TypeFerenceException($"Embedding cycle detected at {id}");
        if (current.Embeds.Count != current.Embeds.Distinct(StringComparer.Ordinal).Count())
            throw new TypeFerenceException($"{id}: an agent cannot embed the same agent more than once");

        try
        {
            var embedded = current.Embeds.Select(x => ResolveAgent(x, visiting)).ToArray();
            var (slots, slotDepths) = MergeSlots(id, current, embedded);
            var norms = Distinct(embedded.SelectMany(x => x.WorkingNorms).Concat(current.WorkingNorms));
            var contexts = Distinct(embedded.SelectMany(x => x.ContextFiles).Concat(current.ContextFiles).Select(NormalizePath));
            var (skills, skillDepths) = MergeSkills(id, current, embedded, contexts);
            var satisfies = resources.Values
                .Where(x => x.Kind == "interface")
                .OrderBy(x => x.Id, StringComparer.Ordinal)
                .Where(x => Satisfies(ResolveInterface(x.Id, new HashSet<string>(StringComparer.Ordinal)), slots, skills))
                .Select(x => x.Id)
                .ToArray();

            var provenance = embedded.SelectMany(x => x.Provenance).ToList();
            provenance.AddRange(current.Embeds.Select(x => new ProvenanceEntry($"embeds.{x}", id)));
            if (!string.IsNullOrWhiteSpace(current.DisplayName)) provenance.Add(new("displayName", id));
            if (!string.IsNullOrWhiteSpace(current.Description)) provenance.Add(new("description", id));
            provenance.AddRange(current.Slots.Keys.Select(x => new ProvenanceEntry($"slots.{x}", id)));
            provenance.AddRange(current.WorkingNorms.Select(_ => new ProvenanceEntry("workingNorms", id)));
            provenance.AddRange(current.ContextFiles.Select(_ => new ProvenanceEntry("contextFiles", id)));
            provenance.AddRange(satisfies.Select(x => new ProvenanceEntry($"satisfies.{x}", id)));

            var resolved = new ResolvedAgent
            {
                Id = id,
                DisplayName = string.IsNullOrWhiteSpace(current.DisplayName) ? id : current.DisplayName,
                Description = current.Description,
                Emit = current.Emit,
                Embeds = current.Embeds.ToArray(),
                Satisfies = satisfies,
                Slots = slots,
                WorkingNorms = norms,
                ContextFiles = contexts,
                Skills = skills.Values.OrderBy(x => x.ContractId, StringComparer.Ordinal).Select(x => x.WithDispatch(id)).ToArray(),
                Provenance = provenance
            };
            _slotDepths[id] = slotDepths;
            _skillDepths[id] = skillDepths;
            return _agentCache[id] = resolved;
        }
        finally
        {
            visiting.Remove(id);
        }
    }

    private (SortedDictionary<string, string> Values, IReadOnlyDictionary<string, int> Depths) MergeSlots(
        string id,
        ResourceDocument current,
        ResolvedAgent[] embedded)
    {
        var result = new SortedDictionary<string, string>(StringComparer.Ordinal);
        var depths = new Dictionary<string, int>(StringComparer.Ordinal);
        foreach (var group in embedded.SelectMany(x => x.Slots.Select(pair =>
                     (Agent: x.Id, pair.Key, pair.Value, Depth: _slotDepths[x.Id][pair.Key] + 1)))
                     .GroupBy(x => x.Key, StringComparer.Ordinal))
        {
            var depth = group.Min(x => x.Depth);
            var nearest = group.Where(x => x.Depth == depth).ToArray();
            if (nearest.Length > 1 && !current.Slots.ContainsKey(group.Key))
                throw new TypeFerenceException($"{id}: embedded slot '{group.Key}' is ambiguous between {string.Join(", ", nearest.Select(x => x.Agent))}; declare it on {id} to resolve the conflict");
            result[group.Key] = nearest[0].Value;
            depths[group.Key] = depth;
        }
        foreach (var pair in current.Slots)
        {
            result[pair.Key] = NormalizePath(pair.Value);
            depths[pair.Key] = 0;
        }
        return (result, depths);
    }

    private (Dictionary<string, ResolvedSkill> Values, IReadOnlyDictionary<string, int> Depths) MergeSkills(
        string id,
        ResourceDocument current,
        ResolvedAgent[] embedded,
        IReadOnlyList<string> contexts)
    {
        var result = new Dictionary<string, ResolvedSkill>(StringComparer.Ordinal);
        var depths = new Dictionary<string, int>(StringComparer.Ordinal);
        var localContracts = current.Skills.Select(x => x.Contract ?? x.Ref).ToHashSet(StringComparer.Ordinal);
        if (localContracts.Count != current.Skills.Count)
            throw new TypeFerenceException($"{id}: a skill contract cannot be declared more than once");
        foreach (var group in embedded.SelectMany(x => x.Skills.Select(skill =>
                     (Agent: x.Id, Skill: skill, Depth: _skillDepths[x.Id][skill.ContractId] + 1)))
                     .GroupBy(x => x.Skill.ContractId, StringComparer.Ordinal))
        {
            var depth = group.Min(x => x.Depth);
            var nearest = group.Where(x => x.Depth == depth).ToArray();
            if (nearest.Length > 1 && !localContracts.Contains(group.Key))
                throw new TypeFerenceException($"{id}: embedded skill contract '{group.Key}' is ambiguous between {string.Join(", ", nearest.Select(x => x.Agent))}; declare the contract on {id} to resolve the conflict");
            result[group.Key] = nearest[0].Skill;
            depths[group.Key] = depth;
        }

        foreach (var binding in current.Skills)
        {
            var implementation = Require(binding.Ref, "skill");
            var contract = binding.Contract ?? binding.Ref;
            if (binding.Contract is not null)
            {
                var contractResource = Require(contract, "skill");
                EnsureSameContract(contractResource, implementation, id);
            }
            if (result.TryGetValue(contract, out var promoted)) EnsureSameContract(promoted, implementation, id);
            result[contract] = new ResolvedSkill
            {
                ContractId = contract,
                ImplementationId = implementation.Id,
                Description = implementation.Description,
                Instructions = implementation.Instructions,
                InputSchema = CanonicalJson(implementation.InputSchema),
                OutputSchema = CanonicalJson(implementation.OutputSchema),
                ContextFiles = Distinct(contexts.Concat(implementation.ContextFiles.Select(NormalizePath))),
                Provenance = [new("skill.contract", contract), new("skill.implementation", implementation.Id)]
            };
            depths[contract] = 0;
        }
        return (result, depths);
    }

    private InterfaceContract ResolveInterface(string id, HashSet<string> visiting)
    {
        if (_interfaceCache.TryGetValue(id, out var cached)) return cached;
        var current = Require(id, "interface");
        if (!visiting.Add(id)) throw new TypeFerenceException($"Interface embedding cycle detected at {id}");
        try
        {
            var embedded = current.Embeds.Select(x => ResolveInterface(x, visiting)).ToArray();
            foreach (var skill in current.RequiresSkills) Require(skill, "skill");
            return _interfaceCache[id] = new(
                Distinct(embedded.SelectMany(x => x.Slots).Concat(current.RequiresSlots)),
                Distinct(embedded.SelectMany(x => x.Skills).Concat(current.RequiresSkills)));
        }
        finally
        {
            visiting.Remove(id);
        }
    }

    private static bool Satisfies(InterfaceContract contract, IReadOnlyDictionary<string, string> slots, IReadOnlyDictionary<string, ResolvedSkill> skills) =>
        contract.Slots.All(slots.ContainsKey) && contract.Skills.All(skills.ContainsKey);

    private ResourceDocument Require(string id, string kind)
    {
        if (!resources.TryGetValue(id, out var resource) || resource.Kind != kind) throw new TypeFerenceException($"Missing {kind}: {id}");
        return resource;
    }

    private static void EnsureSameContract(ResolvedSkill contract, ResourceDocument implementation, string agent)
    {
        if (contract.InputSchema != CanonicalJson(implementation.InputSchema) || contract.OutputSchema != CanonicalJson(implementation.OutputSchema))
            throw new TypeFerenceException($"{agent}: implementation {implementation.Id} changes the public contract of {contract.ContractId}");
    }

    private static void EnsureSameContract(ResourceDocument contract, ResourceDocument implementation, string agent)
    {
        if (CanonicalJson(contract.InputSchema) != CanonicalJson(implementation.InputSchema) ||
            CanonicalJson(contract.OutputSchema) != CanonicalJson(implementation.OutputSchema))
            throw new TypeFerenceException($"{agent}: implementation {implementation.Id} changes the public contract of {contract.Id}");
    }

    private static string NormalizePath(string value) => value.Replace('\\', '/').TrimStart('/');
    private static IReadOnlyList<string> Distinct(IEnumerable<string> values) => values.Distinct(StringComparer.Ordinal).ToArray();
    private static string CanonicalJson(string json) => JsonSerializer.Serialize(JsonDocument.Parse(json).RootElement);
    private sealed record InterfaceContract(IReadOnlyList<string> Slots, IReadOnlyList<string> Skills);
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

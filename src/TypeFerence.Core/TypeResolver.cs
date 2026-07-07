using System.Text.Json;

namespace TypeFerence.Core;

public sealed class TypeResolver(IReadOnlyDictionary<string, ResourceDocument> resources)
{
    private readonly Dictionary<string, ResolvedAgent> _componentCache = new(StringComparer.Ordinal);
    private readonly Dictionary<string, InterfaceContract> _interfaceCache = new(StringComparer.Ordinal);
    private readonly Dictionary<string, IReadOnlyDictionary<string, int>> _slotDepths = new(StringComparer.Ordinal);
    private readonly Dictionary<string, IReadOnlyDictionary<string, int>> _skillDepths = new(StringComparer.Ordinal);

    public IReadOnlyList<ResolvedAgent> ResolveAll()
    {
        foreach (var item in resources.Values.Where(x => x.Kind == "skill").OrderBy(x => x.Id, StringComparer.Ordinal))
            ValidateSkillImplementation(item);
        foreach (var item in resources.Values.Where(x => x.Kind == "interface").OrderBy(x => x.Id, StringComparer.Ordinal))
            ResolveInterface(item.Id, new HashSet<string>(StringComparer.Ordinal));
        foreach (var item in resources.Values.Where(x => x.Kind == "profile").OrderBy(x => x.Id, StringComparer.Ordinal))
            ResolveComponent(item.Id, new HashSet<string>(StringComparer.Ordinal), requireAgent: false);
        return resources.Values
            .Where(x => x.Kind == "agent")
            .OrderBy(x => x.Id, StringComparer.Ordinal)
            .Select(x => Resolve(x.Id))
            .ToArray();
    }

    public ResolvedAgent Resolve(string id) => ResolveComponent(id, new HashSet<string>(StringComparer.Ordinal), requireAgent: true);

    private ResolvedAgent ResolveComponent(string id, HashSet<string> visiting, bool requireAgent)
    {
        if (requireAgent) Require(id, "agent");
        if (_componentCache.TryGetValue(id, out var cached)) return cached;
        var current = RequireEmbeddable(id);
        if (!visiting.Add(id)) throw new TypeFerenceException($"Embedding cycle detected at {id}");
        if (current.Embeds.Count != current.Embeds.Distinct(StringComparer.Ordinal).Count())
            throw new TypeFerenceException($"{id}: a {current.Kind} cannot embed the same resource more than once");

        try
        {
            var embedded = current.Embeds.Select(x =>
            {
                var embeddedResource = RequireEmbeddable(x);
                if (current.Kind == "profile" && embeddedResource.Kind != "profile")
                    throw new TypeFerenceException($"{id}: profiles can only embed profiles");
                return ResolveComponent(x, visiting, requireAgent: false);
            }).ToArray();
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

            var provenance = embedded.SelectMany(x => x.Provenance.Where(IsPromotedProvenance)).ToList();
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
                Skills = skills.Values.OrderBy(x => x.CapabilityId, StringComparer.Ordinal).Select(x => x.WithDispatch(id)).ToArray(),
                Provenance = provenance
            };
            _slotDepths[id] = slotDepths;
            _skillDepths[id] = skillDepths;
            return _componentCache[id] = resolved;
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
        var localCapabilities = current.Skills.Select(x => ResolveCapabilityId(x, id)).ToHashSet(StringComparer.Ordinal);
        if (localCapabilities.Count != current.Skills.Count)
            throw new TypeFerenceException($"{id}: a capability cannot be bound more than once");
        foreach (var group in embedded.SelectMany(x => x.Skills.Select(skill =>
                     (Agent: x.Id, Skill: skill, Depth: _skillDepths[x.Id][skill.CapabilityId] + 1)))
                     .GroupBy(x => x.Skill.CapabilityId, StringComparer.Ordinal))
        {
            var depth = group.Min(x => x.Depth);
            var nearest = group.Where(x => x.Depth == depth).ToArray();
            if (nearest.Length > 1 && !localCapabilities.Contains(group.Key))
                throw new TypeFerenceException($"{id}: embedded capability '{group.Key}' is ambiguous between {string.Join(", ", nearest.Select(x => x.Agent))}; bind the capability on {id} to resolve the conflict");
            result[group.Key] = nearest[0].Skill;
            depths[group.Key] = depth;
        }

        foreach (var binding in current.Skills)
        {
            var implementation = Require(binding.Ref, "skill");
            var capabilityId = ResolveCapabilityId(binding, id);
            var capability = Require(capabilityId, "capability");
            EnsureImplementsCapability(capability, implementation, id);
            if (result.TryGetValue(capabilityId, out var promoted)) EnsureSameCapability(promoted, capability, id);
            result[capabilityId] = new ResolvedSkill
            {
                CapabilityId = capabilityId,
                ImplementationId = implementation.Id,
                Description = implementation.Description,
                Instructions = implementation.Instructions,
                InputSchema = CanonicalJson(implementation.InputSchema),
                OutputSchema = CanonicalJson(implementation.OutputSchema),
                ContextFiles = Distinct(contexts.Concat(implementation.ContextFiles.Select(NormalizePath))),
                Provenance = [new("skill.capability", capabilityId), new("skill.implementation", implementation.Id)]
            };
            depths[capabilityId] = 0;
        }
        return (result, depths);
    }

    private string ResolveCapabilityId(SkillBinding binding, string agent)
    {
        var implementation = Require(binding.Ref, "skill");
        if (string.IsNullOrWhiteSpace(implementation.Binds))
            throw new TypeFerenceException($"{agent}: skill {implementation.Id} does not bind a capability");
        if (binding.Capability is not null && binding.Capability != implementation.Binds)
            throw new TypeFerenceException($"{agent}: binding declares capability {binding.Capability}, but skill {implementation.Id} binds {implementation.Binds}");
        return binding.Capability ?? implementation.Binds;
    }

    private void ValidateSkillImplementation(ResourceDocument implementation)
    {
        if (string.IsNullOrWhiteSpace(implementation.Binds))
            throw new TypeFerenceException($"Skill {implementation.Id} does not bind a capability");
        var capability = Require(implementation.Binds, "capability");
        EnsureImplementsCapability(capability, implementation, implementation.Id);
    }

    private InterfaceContract ResolveInterface(string id, HashSet<string> visiting)
    {
        if (_interfaceCache.TryGetValue(id, out var cached)) return cached;
        var current = Require(id, "interface");
        if (!visiting.Add(id)) throw new TypeFerenceException($"Interface embedding cycle detected at {id}");
        try
        {
            var embedded = current.Embeds.Select(x => ResolveInterface(x, visiting)).ToArray();
            foreach (var capability in current.RequiresCapabilities) Require(capability, "capability");
            return _interfaceCache[id] = new(
                Distinct(embedded.SelectMany(x => x.Slots).Concat(current.RequiresSlots)),
                Distinct(embedded.SelectMany(x => x.Skills).Concat(current.RequiresCapabilities)));
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

    private ResourceDocument RequireEmbeddable(string id)
    {
        if (!resources.TryGetValue(id, out var resource) || resource.Kind is not ("agent" or "profile"))
            throw new TypeFerenceException($"Missing embeddable resource: {id}");
        return resource;
    }

    private static void EnsureSameCapability(ResolvedSkill promoted, ResourceDocument capability, string agent)
    {
        if (promoted.InputSchema != CanonicalJson(capability.InputSchema) || promoted.OutputSchema != CanonicalJson(capability.OutputSchema))
            throw new TypeFerenceException($"{agent}: promoted implementation {promoted.ImplementationId} changes the public contract of {capability.Id}");
    }

    private static void EnsureImplementsCapability(ResourceDocument capability, ResourceDocument implementation, string agent)
    {
        if (implementation.Binds != capability.Id)
            throw new TypeFerenceException($"{agent}: implementation {implementation.Id} binds {implementation.Binds}, not capability {capability.Id}");
        if (CanonicalJson(capability.InputSchema) != CanonicalJson(implementation.InputSchema) ||
            CanonicalJson(capability.OutputSchema) != CanonicalJson(implementation.OutputSchema))
            throw new TypeFerenceException($"{agent}: implementation {implementation.Id} changes the public contract of {capability.Id}");
    }

    private static string NormalizePath(string value) => value.Replace('\\', '/').TrimStart('/');
    private static IReadOnlyList<string> Distinct(IEnumerable<string> values) => values.Distinct(StringComparer.Ordinal).ToArray();
    private static string CanonicalJson(string json) => JsonSerializer.Serialize(JsonDocument.Parse(json).RootElement);
    private static bool IsPromotedProvenance(ProvenanceEntry entry) =>
        entry.Field is not ("displayName" or "description") &&
        !entry.Field.StartsWith("satisfies.", StringComparison.Ordinal);
    private sealed record InterfaceContract(IReadOnlyList<string> Slots, IReadOnlyList<string> Skills);
}

internal static class ResolvedSkillExtensions
{
    internal static ResolvedSkill WithDispatch(this ResolvedSkill skill, string agentId) => new()
    {
        DispatchName = $"{agentId.Split('/').Last().Split('@')[0]}.{skill.CapabilityId.Split('/').Last().Split('@')[0]}",
        CapabilityId = skill.CapabilityId,
        ImplementationId = skill.ImplementationId,
        Description = skill.Description,
        Instructions = skill.Instructions,
        InputSchema = skill.InputSchema,
        OutputSchema = skill.OutputSchema,
        ContextFiles = skill.ContextFiles,
        Provenance = skill.Provenance
    };
}

using System.Text.Json;
using System.Text.Json.Nodes;
using System.Diagnostics;
using ModelContextProtocol.Client;
using ModelContextProtocol.Protocol;
using TypeFerence.Core;
using Xunit;

namespace TypeFerence.Tests;

public sealed class CompilerTests
{
    private static string Root => FindRoot();
    private static string Example => Path.Combine(Root, "examples", "helio");

    [Fact]
    public void Profiles_AreReusableAndNotEmittedAsAgents()
    {
        var agents = new TypeFerenceCompiler().Validate(Example);
        Assert.Equal(2, agents.Count);
        Assert.DoesNotContain(agents, x => x.Id == "helio/profiles/enterprise-defaults@1.0.0");
        Assert.All(agents, x => Assert.True(x.Emit));
        Assert.All(agents, x => Assert.Contains("helio/profiles/", x.Embeds.Single()));
    }

    [Fact]
    public void ConcreteAgents_PromoteEmbeddedBehavior()
    {
        var agents = new TypeFerenceCompiler().Validate(Example).Where(x => x.Emit);
        Assert.All(agents, x => Assert.Contains("organization", x.Slots.Keys));
    }

    [Fact]
    public void Override_PreservesCapabilityAndUsesDerivedDispatch()
    {
        var agent = new TypeFerenceCompiler().Validate(Example).Single(x => x.Id == "helio/payments-repo-agent@1.0.0");
        var skill = Assert.Single(agent.Skills);
        Assert.Equal("helio/capabilities/repository-status@1.0.0", skill.CapabilityId);
        Assert.Equal("helio/skills/payments-repository-status@1.0.0", skill.ImplementationId);
        Assert.Equal("payments-repo-agent.repository-status", skill.DispatchName);
    }

    [Fact]
    public void InvocationPackage_ContainsResolvedProvenance()
    {
        var agent = new TypeFerenceCompiler().Validate(Example).Single(x => x.Id == "helio/payments-repo-agent@1.0.0");
        using var args = JsonDocument.Parse("{\"focus\":\"release\"}");
        var package = TypeFerenceCompiler.Invoke(agent, agent.Skills.Single(), args.RootElement);
        Assert.Equal("payments-repo-agent.repository-status", package.DispatchName);
        Assert.Contains(package.ContextReferences, x => x.EndsWith("payments-service.md", StringComparison.Ordinal));
        Assert.Contains(package.Provenance, x => x.Field == "skill.implementation");
    }

    [Fact]
    public void Compilation_IsDeterministicAndProducesNativeShapes()
    {
        using var one = new TempDirectory();
        using var two = new TempDirectory();
        var compiler = new TypeFerenceCompiler();
        compiler.Build(Example, one.Path, Enum.GetValues<CompilationTarget>(), new ArdPublicationOptions("helio.example"));
        compiler.Build(Example, two.Path, Enum.GetValues<CompilationTarget>(), new ArdPublicationOptions("helio.example"));
        Assert.Equal("a22f5410ba5f8e172c8d25dd2ff3efb867b212753723fd2e3b1faca21d2b3963", TypeFerenceCompiler.HashDirectory(one.Path));
        Assert.Equal(TypeFerenceCompiler.HashDirectory(one.Path), TypeFerenceCompiler.HashDirectory(two.Path));
        Assert.False(DiffResult.Compare(one.Path, two.Path).Different);
        Assert.True(File.Exists(Path.Combine(one.Path, "codex", "executive-assistant", ".agents", "skills", "prepare-brief", "SKILL.md")));
        Assert.True(File.Exists(Path.Combine(one.Path, "codex", "executive-assistant", ".typeference", "bundle.json")));
        Assert.True(File.Exists(Path.Combine(one.Path, "copilot", "executive-assistant", ".github", "agents", "executive-assistant.agent.md")));
        Assert.True(File.Exists(Path.Combine(one.Path, "copilot", "executive-assistant", ".typeference", "bundle.json")));
        Assert.True(File.Exists(Path.Combine(one.Path, "cursor", "executive-assistant", ".cursor", "rules", "executive-assistant.mdc")));
        Assert.True(File.Exists(Path.Combine(one.Path, "cursor", "executive-assistant", ".typeference", "bundle.json")));
        var catalogPath = Path.Combine(one.Path, "ard", "ai-catalog.json");
        Assert.True(File.Exists(catalogPath));
        using var catalog = JsonDocument.Parse(File.ReadAllText(catalogPath));
        Assert.Equal("1.0", catalog.RootElement.GetProperty("specVersion").GetString());
        var entries = catalog.RootElement.GetProperty("entries");
        Assert.Equal(9, entries.GetArrayLength());
        Assert.Single(entries.EnumerateArray(), entry =>
            entry.GetProperty("type").GetString() == "application/vnd.typeference.source-package+json");
        var targetEntries = entries.EnumerateArray().Where(entry =>
            entry.GetProperty("type").GetString() == "application/vnd.typeference.target-bundle+json").ToArray();
        Assert.Equal(8, targetEntries.Length);
        Assert.All(targetEntries, entry =>
        {
            Assert.True(entry.TryGetProperty("data", out var data));
            Assert.Contains(data.GetProperty("target").GetString(), new[] { "neutral", "codex", "copilot", "cursor" });
            Assert.Equal("derivedFrom", entry.GetProperty("trustManifest").GetProperty("provenance")[0].GetProperty("relation").GetString());
        });
        File.AppendAllText(Path.Combine(two.Path, "neutral", "executive-assistant", "AGENTS.md"), "changed");
        Assert.True(DiffResult.Compare(one.Path, two.Path).Different);
    }

    [Fact]
    public void TrustConfiguration_EnrichesSourceAndBundlesDeterministically()
    {
        using var source = new TempDirectory();
        using var one = new TempDirectory();
        using var two = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration());

        var compiler = new TypeFerenceCompiler();
        compiler.Build(source.Path, one.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example"));
        compiler.Build(source.Path, two.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example"));
        Assert.Equal(TypeFerenceCompiler.HashDirectory(one.Path), TypeFerenceCompiler.HashDirectory(two.Path));
        Assert.False(DiffResult.Compare(one.Path, two.Path).Different);

        using var catalog = JsonDocument.Parse(File.ReadAllText(Path.Combine(one.Path, "ard", "ai-catalog.json")));
        var entries = catalog.RootElement.GetProperty("entries").EnumerateArray().ToArray();
        var sourceEntry = Assert.Single(entries, x => x.GetProperty("type").GetString() == "application/vnd.typeference.source-package+json");
        var sourceTrust = sourceEntry.GetProperty("trustManifest");
        Assert.Equal("did:web:helio.example:typeference:source:helio", sourceTrust.GetProperty("identity").GetString());
        Assert.Equal("https://slsa.dev/provenance/v1", sourceTrust.GetProperty("attestations")[0].GetProperty("type").GetString());
        Assert.Equal("external", sourceTrust.GetProperty("metadata")
            .GetProperty("com.github.buchk.typeference.signatureIntent").GetProperty("status").GetString());

        var bundles = entries.Where(x => x.GetProperty("type").GetString() == "application/vnd.typeference.target-bundle+json").ToArray();
        Assert.Equal(2, bundles.Length);
        Assert.All(bundles, entry =>
        {
            var trust = entry.GetProperty("trustManifest");
            Assert.StartsWith("spiffe://helio.example/typeference/neutral/", trust.GetProperty("identity").GetString(), StringComparison.Ordinal);
            Assert.Equal("spiffe", trust.GetProperty("identityType").GetString());
            Assert.Equal("urn:trust:helio-agent-governance-v1", trust.GetProperty("trustSchema").GetProperty("identifier").GetString());
            Assert.Equal("derivedFrom", trust.GetProperty("provenance")[0].GetProperty("relation").GetString());
            Assert.Equal("publishedFrom", trust.GetProperty("provenance")[1].GetProperty("relation").GetString());
            var metadata = trust.GetProperty("metadata");
            Assert.Equal("typeference-directory-v1", metadata.GetProperty("com.github.buchk.typeference.artifactDigest").GetProperty("scheme").GetString());
            Assert.Equal("sha256:" + new string('c', 64), metadata.GetProperty("com.helio.verification").GetProperty("policyDigest").GetString());
        });
    }

    [Fact]
    public void ExternalDetachedJwsSignatures_AreImportedWithoutKeys()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        using var unsignedOutput = new TempDirectory();
        using var signatureDirectory = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration());
        new TypeFerenceCompiler().Build(
            source.Path, unsignedOutput.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example"));
        var signaturesPath = Path.Combine(signatureDirectory.Path, "signatures.json");
        File.WriteAllText(signaturesPath, """
{
  "urn:air:helio.example:typeference:neutral:executive-assistant": "eyJhbGciOiJFUzI1NiJ9..c2ln",
  "urn:air:helio.example:typeference:neutral:payments-repo-agent": "eyJhbGciOiJFUzI1NiJ9..c2ln"
}
""");

        new TypeFerenceCompiler().Build(
            source.Path,
            output.Path,
            [CompilationTarget.Neutral],
            new ArdPublicationOptions("helio.example", TrustSignaturesPath: signaturesPath));
        using var catalog = JsonDocument.Parse(File.ReadAllText(Path.Combine(output.Path, "ard", "ai-catalog.json")));
        var unsignedCatalog = JsonNode.Parse(File.ReadAllText(Path.Combine(unsignedOutput.Path, "ard", "ai-catalog.json")))!;
        var unsignedManifests = unsignedCatalog["entries"]!.AsArray()
            .Where(x => x!["type"]!.GetValue<string>() == "application/vnd.typeference.target-bundle+json")
            .ToDictionary(x => x!["identifier"]!.GetValue<string>(), x => x!["trustManifest"]!.DeepClone(), StringComparer.Ordinal);
        var bundles = catalog.RootElement.GetProperty("entries").EnumerateArray()
            .Where(x => x.GetProperty("type").GetString() == "application/vnd.typeference.target-bundle+json");
        Assert.All(bundles, entry =>
        {
            var trust = entry.GetProperty("trustManifest");
            Assert.Equal("eyJhbGciOiJFUzI1NiJ9..c2ln", trust.GetProperty("signature").GetString());
            Assert.Equal("external", trust.GetProperty("metadata")
                .GetProperty("com.github.buchk.typeference.signatureIntent").GetProperty("status").GetString());
            var signedPayload = JsonNode.Parse(trust.GetRawText())!.AsObject();
            signedPayload.Remove("signature");
            Assert.True(JsonNode.DeepEquals(unsignedManifests[entry.GetProperty("identifier").GetString()!], signedPayload));
        });
    }

    [Fact]
    public void TrustSignaturesInsideSource_AreRejectedToPreventCycles()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration());
        var signatures = Path.Combine(source.Path, "signatures.json");
        File.WriteAllText(signatures, "{}");
        var exception = Assert.Throws<TypeFerenceException>(() => new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example", TrustSignaturesPath: signatures)));
        Assert.Contains("digest/signature cycle", exception.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("sha256:abc")]
    [InlineData("SHA256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")]
    public void TrustConfiguration_RejectsMalformedDigests(string digest)
    {
        using var source = new TempDirectory();
        CopyDirectory(Example, source.Path);
        var valid = "sha256:" + new string('a', 64);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration().Replace(valid, digest, StringComparison.Ordinal));
        var exception = Assert.Throws<TypeFerenceException>(() => new TypeFerenceCompiler().Validate(source.Path));
        Assert.Contains("lowercase SHA-256", exception.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void TrustSignatureMap_RejectsUnknownEntries()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        using var signatureDirectory = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration());
        var signatures = Path.Combine(signatureDirectory.Path, "signatures.json");
        File.WriteAllText(signatures, "{\"urn:air:helio.example:typeference:neutral:unknown\":\"e30..c2ln\"}");
        var exception = Assert.Throws<TypeFerenceException>(() => new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example", TrustSignaturesPath: signatures)));
        Assert.Contains("does not match", exception.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void RequiredTrustSignatures_MustCoverEveryConfiguredEntry()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration(bundleSignatureRequired: true));
        var exception = Assert.Throws<TypeFerenceException>(() => new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example")));
        Assert.Contains("signature is required", exception.Message, StringComparison.OrdinalIgnoreCase);

        new TypeFerenceCompiler().Build(
            source.Path,
            output.Path,
            [CompilationTarget.Neutral],
            new ArdPublicationOptions("helio.example", AllowUnsignedTrust: true));
        using var catalog = JsonDocument.Parse(File.ReadAllText(Path.Combine(output.Path, "ard", "ai-catalog.json")));
        var bundles = catalog.RootElement.GetProperty("entries").EnumerateArray()
            .Where(x => x.GetProperty("type").GetString() == "application/vnd.typeference.target-bundle+json");
        Assert.All(bundles, x => Assert.False(x.GetProperty("trustManifest").TryGetProperty("signature", out _)));
    }

    [Theory]
    [InlineData("spiffe://other.example/typeference/{target}/{agent}", "does not align")]
    [InlineData("spiffe://helio.example/typeference/{agent}", "must contain {agent} and {target}")]
    public void InvalidTrustIdentities_AreRejected(string identityTemplate, string expected)
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), TrustConfiguration(identityTemplate: identityTemplate));
        var exception = Assert.Throws<TypeFerenceException>(() => new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example")));
        Assert.Contains(expected, exception.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("dns:other.example", "dns", "does not align")]
    [InlineData("did:key:z6MkhExample", "did", "does not expose")]
    public void TrustIdentity_MustExposeThePublisherDomain(string identity, string identityType, string expected)
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), SourceTrustConfiguration(identity, identityType));
        var exception = Assert.Throws<TypeFerenceException>(() => new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example")));
        Assert.Contains(expected, exception.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void CustomIdentityTypeHints_RemainExtensible()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        var configuration = TrustConfiguration().Replace("identityType: spiffe", "identityType: helio-workload-v1", StringComparison.Ordinal);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), configuration);
        new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example"));
        using var catalog = JsonDocument.Parse(File.ReadAllText(Path.Combine(output.Path, "ard", "ai-catalog.json")));
        var bundle = catalog.RootElement.GetProperty("entries").EnumerateArray()
            .First(x => x.GetProperty("type").GetString() == "application/vnd.typeference.target-bundle+json");
        Assert.Equal("helio-workload-v1", bundle.GetProperty("trustManifest").GetProperty("identityType").GetString());
    }

    [Fact]
    public void DnsIdentity_CanBindToPublisherDomain()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), SourceTrustConfiguration("dns:helio.example", "dns"));
        new TypeFerenceCompiler().Build(
            source.Path, output.Path, [CompilationTarget.Neutral], new ArdPublicationOptions("helio.example"));
        using var catalog = JsonDocument.Parse(File.ReadAllText(Path.Combine(output.Path, "ard", "ai-catalog.json")));
        Assert.Equal("dns:helio.example", catalog.RootElement.GetProperty("entries")[0]
            .GetProperty("trustManifest").GetProperty("identity").GetString());
    }

    [Fact]
    public async Task CliBuild_AcceptsExplicitTrustConfiguration()
    {
        using var source = new TempDirectory();
        using var output = new TempDirectory();
        CopyDirectory(Example, source.Path);
        var trustPath = Path.Combine(source.Path, "enterprise.trust.yaml");
        File.WriteAllText(trustPath, TrustConfiguration());
        File.WriteAllText(Path.Combine(source.Path, "typeference.trust.yaml"), SourceTrustConfiguration("did:web:other.example:unused", "did"));
        Assert.Equal(0, await RunCli("validate", source.Path, "--trust-config", trustPath));
        Assert.Equal(0, await RunCli("build", source.Path, "--target", "neutral", "--out", output.Path,
            "--emit-ard", "--publisher-domain", "helio.example", "--trust-config", trustPath));
        using var catalog = JsonDocument.Parse(File.ReadAllText(Path.Combine(output.Path, "ard", "ai-catalog.json")));
        Assert.All(catalog.RootElement.GetProperty("entries").EnumerateArray(), x => Assert.True(x.TryGetProperty("trustManifest", out _)));
    }

    [Theory]
    [InlineData("--trust-config")]
    [InlineData("--trust-signatures")]
    public async Task CliTrustOptions_RequireValues(string option)
    {
        using var output = new TempDirectory();
        Assert.Equal(2, await RunCli("build", Example, "--target", "neutral", "--out", output.Path,
            "--emit-ard", "--publisher-domain", "helio.example", option));
    }

    [Fact]
    public void EmbeddingCycles_AreRejected()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/a@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/a@1.0.0", Embeds = ["test/b@1.0.0"] },
            ["test/b@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/b@1.0.0", Embeds = ["test/a@1.0.0"] }
        };
        Assert.Contains("cycle", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void MultipleEmbedding_RequiresExplicitConflictResolution()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/left@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/left@1.0.0", Slots = { ["owner"] = "left.md" } },
            ["test/right@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/right@1.0.0", Slots = { ["owner"] = "right.md" } },
            ["test/ambiguous@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/ambiguous@1.0.0",
                Embeds = ["test/left@1.0.0", "test/right@1.0.0"]
            }
        };
        Assert.Contains("ambiguous", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).Resolve("test/ambiguous@1.0.0")).Message, StringComparison.OrdinalIgnoreCase);

        resources["test/ambiguous@1.0.0"].Slots["owner"] = "outer.md";
        Assert.Equal("outer.md", new TypeResolver(resources).Resolve("test/ambiguous@1.0.0").Slots["owner"]);
    }

    [Fact]
    public void MultipleEmbedding_PrefersTheShallowestPromotedMember()
    {
        var capability = new ResourceDocument
        {
            SchemaVersion = 3,
            Kind = "capability",
            Id = "test/capabilities/status@1.0.0"
        };
        var baseSkill = new ResourceDocument
        {
            SchemaVersion = 3,
            Kind = "skill",
            Id = "test/skills/status@1.0.0",
            Binds = capability.Id
        };
        var specializedSkill = new ResourceDocument
        {
            SchemaVersion = 3,
            Kind = "skill",
            Id = "test/skills/special-status@1.0.0",
            Binds = capability.Id
        };
        var resources = new Dictionary<string, ResourceDocument>
        {
            [capability.Id] = capability,
            [baseSkill.Id] = baseSkill,
            [specializedSkill.Id] = specializedSkill,
            ["test/deep@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/deep@1.0.0",
                Slots = { ["owner"] = "deep.md" },
                Skills = [new SkillBinding { Ref = baseSkill.Id }]
            },
            ["test/middle@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/middle@1.0.0",
                Embeds = ["test/deep@1.0.0"]
            },
            ["test/shallow@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/shallow@1.0.0",
                Slots = { ["owner"] = "shallow.md" },
                Skills = [new SkillBinding { Ref = specializedSkill.Id, Capability = capability.Id }]
            },
            ["test/outer@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/outer@1.0.0",
                Embeds = ["test/middle@1.0.0", "test/shallow@1.0.0"]
            }
        };

        var resolved = new TypeResolver(resources).Resolve("test/outer@1.0.0");
        Assert.Equal("shallow.md", resolved.Slots["owner"]);
        Assert.Equal(specializedSkill.Id, Assert.Single(resolved.Skills).ImplementationId);
    }

    [Fact]
    public void Provenance_CoversEffectiveScalarsStructuralContractsAndSkills()
    {
        var agent = new TypeFerenceCompiler().Validate(Example).Single(x => x.Id == "helio/executive-assistant@1.0.0");
        Assert.Contains(agent.Provenance, x => x.Field == "displayName" && x.Source == agent.Id);
        Assert.Contains(agent.Provenance, x => x.Field.StartsWith("satisfies.", StringComparison.Ordinal));
        Assert.All(agent.Skills, skill =>
        {
            Assert.Contains(skill.Provenance, x => x.Field == "skill.capability");
            Assert.Contains(skill.Provenance, x => x.Field == "skill.implementation");
        });
    }

    [Fact]
    public void Interfaces_AreSatisfiedImplicitly()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/contract@1.0.0"] = new() { SchemaVersion = 3, Kind = "interface", Id = "test/contract@1.0.0", RequiresSlots = ["owner"] },
            ["test/plain@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/plain@1.0.0" },
            ["test/owned@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/owned@1.0.0", Slots = { ["owner"] = "owner.md" } }
        };
        var agents = new TypeResolver(resources).ResolveAll();
        Assert.Empty(agents.Single(x => x.Id == "test/plain@1.0.0").Satisfies);
        Assert.Contains("test/contract@1.0.0", agents.Single(x => x.Id == "test/owned@1.0.0").Satisfies);
    }

    [Fact]
    public void InterfaceCapabilityRequirements_MustReferenceCapabilities()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/contract@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "interface",
                Id = "test/contract@1.0.0",
                RequiresCapabilities = ["test/capabilities/missing@1.0.0"]
            }
        };

        Assert.Contains("Missing capability", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Skills_MustBindACapability()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "skill.yaml"), """
schemaVersion: 3
kind: skill
id: test/skills/status@1.0.0
""");

        Assert.Contains("skills must bind a capability", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void AgentSkillBindings_MustMatchTheSkillCapability()
    {
        var capability = new ResourceDocument { SchemaVersion = 3, Kind = "capability", Id = "test/capabilities/status@1.0.0" };
        var otherCapability = new ResourceDocument { SchemaVersion = 3, Kind = "capability", Id = "test/capabilities/other-status@1.0.0" };
        var skill = new ResourceDocument { SchemaVersion = 3, Kind = "skill", Id = "test/skills/status@1.0.0", Binds = capability.Id };
        var resources = new Dictionary<string, ResourceDocument>
        {
            [capability.Id] = capability,
            [otherCapability.Id] = otherCapability,
            [skill.Id] = skill,
            ["test/agent@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/agent@1.0.0",
                Skills = [new SkillBinding { Ref = skill.Id, Capability = otherCapability.Id }]
            }
        };

        Assert.Contains("binding declares capability", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void MissingEmbeddedResources_AreRejected()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/base@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/base@1.0.0", Embeds = ["test/missing@1.0.0"] }
        };
        Assert.Contains("Missing embeddable resource", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Profiles_CannotEmbedAgents()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/profile@1.0.0"] = new() { SchemaVersion = 3, Kind = "profile", Id = "test/profile@1.0.0", Embeds = ["test/agent@1.0.0"] },
            ["test/agent@1.0.0"] = new() { SchemaVersion = 3, Kind = "agent", Id = "test/agent@1.0.0" }
        };
        Assert.Contains("profiles can only embed profiles", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void SkillCapabilityImplementations_MustPreserveSchemas()
    {
        var capability = new ResourceDocument
        {
            SchemaVersion = 3,
            Kind = "capability",
            Id = "test/capabilities/status@1.0.0",
            InputSchema = "{\"type\":\"object\",\"properties\":{\"focus\":{\"type\":\"string\"}}}",
            OutputSchema = "{\"type\":\"object\"}"
        };
        var baseSkill = new ResourceDocument
        {
            SchemaVersion = 3,
            Kind = "skill",
            Id = "test/skills/status@1.0.0",
            Binds = capability.Id,
            InputSchema = "{\"type\":\"object\",\"properties\":{\"focus\":{\"type\":\"string\"}}}",
            OutputSchema = "{\"type\":\"object\"}"
        };
        var replacement = new ResourceDocument
        {
            SchemaVersion = 3,
            Kind = "skill",
            Id = "test/skills/special-status@1.0.0",
            Binds = capability.Id,
            InputSchema = "{\"type\":\"object\",\"properties\":{\"count\":{\"type\":\"number\"}}}",
            OutputSchema = "{\"type\":\"object\"}"
        };
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/base@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/base@1.0.0",
                Emit = false,
                Skills = [new SkillBinding { Ref = baseSkill.Id }]
            },
            ["test/concrete@1.0.0"] = new()
            {
                SchemaVersion = 3,
                Kind = "agent",
                Id = "test/concrete@1.0.0",
                Embeds = ["test/base@1.0.0"],
                Skills = [new SkillBinding { Ref = replacement.Id, Capability = capability.Id }]
            },
            [capability.Id] = capability,
            [baseSkill.Id] = baseSkill,
            [replacement.Id] = replacement
        };
        Assert.Contains("changes the public contract", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Interfaces_CanEmbedInterfaces()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "base.yaml"), "schemaVersion: 3\nkind: interface\nid: test/base@1.0.0\nrequiresSlots: [owner]\n");
        File.WriteAllText(Path.Combine(source.Path, "interface.yaml"), "schemaVersion: 3\nkind: interface\nid: test/contract@1.0.0\nembeds: [test/base@1.0.0]\nrequiresSlots: [repository]\n");
        File.WriteAllText(Path.Combine(source.Path, "agent.yaml"), "schemaVersion: 3\nkind: agent\nid: test/agent@1.0.0\nslots: { owner: owner.md, repository: repository.md }\n");
        File.WriteAllText(Path.Combine(source.Path, "owner.md"), "owner");
        File.WriteAllText(Path.Combine(source.Path, "repository.md"), "repository");
        var agent = new TypeResolver(new ResourceLoader().Load(source.Path)).Resolve("test/agent@1.0.0");
        Assert.Contains("test/contract@1.0.0", agent.Satisfies);
    }

    [Theory]
    [InlineData("../outside.md", "escapes source root")]
    [InlineData("context/missing.md", "does not exist")]
    public void ContextPaths_MustStayInsideSourceAndExist(string path, string expected)
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "agent.yaml"), $"schemaVersion: 3\nkind: agent\nid: test/agent@1.0.0\ncontextFiles:\n  - {path}\n");
        Assert.Contains(expected, Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void UnknownYamlFields_AreRejected()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "root.yaml"), """
schemaVersion: 3
kind: agent
id: test/agent@1.0.0
unknownField: rejected
""");
        Assert.Contains("invalid YAML", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("extends: test/base@1.0.0")]
    [InlineData("implements: [test/contract@1.0.0]")]
    public void LegacyInheritanceFields_AreRejected(string field)
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "agent.yaml"), $"schemaVersion: 3\nkind: agent\nid: test/agent@1.0.0\n{field}\n");
        Assert.Contains("invalid YAML", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void VersionOneResources_AreRejected()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "agent.yaml"), "schemaVersion: 1\nkind: agent\nid: test/agent@1.0.0\n");
        Assert.Contains("schemaVersion must be 3", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void ResourceIds_RequireSemanticVersions()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "root.yaml"), """
schemaVersion: 3
kind: agent
id: system/object@latest
""");
        Assert.Contains("semantic-version", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public async Task McpServer_ListsDottedToolsAndDispatchesInvocationPackage()
    {
        using var compiled = new TempDirectory();
        new TypeFerenceCompiler().Build(Example, compiled.Path, [CompilationTarget.Neutral]);
        var transport = new StdioClientTransport(new StdioClientTransportOptions
        {
            Name = "TypeFerence test",
            Command = "dotnet",
            Arguments = [CliAssembly, "serve", Path.Combine(compiled.Path, "neutral")],
            WorkingDirectory = Root
        });
        await using var client = await McpClient.CreateAsync(transport);
        var tools = await client.ListToolsAsync();
        Assert.Contains(tools, x => x.Name == "payments-repo-agent.repository-status");
        var result = await client.CallToolAsync(
            "payments-repo-agent.repository-status",
            new Dictionary<string, object?> { ["arguments"] = new { focus = "release" } });
        var text = Assert.Single(result.Content.OfType<TextContentBlock>()).Text;
        Assert.Contains("payments-repo-agent.repository-status", text, StringComparison.Ordinal);
        Assert.Contains("payments-repository-status", text, StringComparison.Ordinal);
    }

    [Fact]
    public async Task DiffCommand_UsesDocumentedExitCodes()
    {
        using var compiled = new TempDirectory();
        using var changed = new TempDirectory();
        CopyDirectory(Example, changed.Path);
        new TypeFerenceCompiler().Build(changed.Path, compiled.Path, [CompilationTarget.Neutral]);
        Assert.Equal(0, await RunCli("diff", changed.Path, "--against", compiled.Path, "--target", "neutral"));
        var agentFile = Path.Combine(changed.Path, "agents", "executive-assistant.agent.yaml");
        await File.AppendAllTextAsync(agentFile, "\nworkingNorms:\n  - Produce an explicit change for the diff test.\n");
        Assert.Equal(1, await RunCli("diff", changed.Path, "--against", compiled.Path, "--target", "neutral"));
    }

    private static string FindRoot()
    {
        var directory = new DirectoryInfo(AppContext.BaseDirectory);
        while (directory is not null && !File.Exists(Path.Combine(directory.FullName, "TypeFerence.slnx"))) directory = directory.Parent;
        return directory?.FullName ?? throw new InvalidOperationException("Repository root not found");
    }

    private static async Task<int> RunCli(params string[] arguments)
    {
        using var process = Process.Start(new ProcessStartInfo("dotnet")
        {
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            WorkingDirectory = Root,
            ArgumentList = { CliAssembly }
        }.WithArguments(arguments)) ?? throw new InvalidOperationException("Could not start TypeFerence CLI");
        await process.WaitForExitAsync();
        return process.ExitCode;
    }

    private static string CliAssembly
    {
        get
        {
            var configuration = new DirectoryInfo(AppContext.BaseDirectory).Parent?.Name
                ?? throw new InvalidOperationException("Could not determine the active build configuration");
            var cli = Path.Combine(Root, "src", "TypeFerence.Cli", "bin", configuration, "net10.0", "typeference.dll");
            return File.Exists(cli) ? cli : throw new InvalidOperationException($"CLI assembly not found for {configuration}: {cli}");
        }
    }

    private static void CopyDirectory(string source, string destination)
    {
        foreach (var directory in Directory.EnumerateDirectories(source, "*", SearchOption.AllDirectories))
            Directory.CreateDirectory(Path.Combine(destination, Path.GetRelativePath(source, directory)));
        foreach (var file in Directory.EnumerateFiles(source, "*", SearchOption.AllDirectories))
            File.Copy(file, Path.Combine(destination, Path.GetRelativePath(source, file)), true);
    }

    private static string TrustConfiguration(string? identityTemplate = null, bool bundleSignatureRequired = false) => $$"""
schemaVersion: 1
source:
  identity: did:web:helio.example:typeference:source:helio
  identityType: did
  attestations:
    - type: https://slsa.dev/provenance/v1
      uri: https://trust.helio.example/provenance/source.intoto.jsonl
      digest: sha256:{{new string('a', 64)}}
  metadata:
    com.helio.governance:
      policy:
        digest: sha256:{{new string('b', 64)}}
  signatureIntent:
    algorithm: ES256
    keyRef: did:web:helio.example#catalog-signing
bundles:
  identityTemplate: {{identityTemplate ?? "spiffe://helio.example/typeference/{target}/{agent}"}}
  identityType: spiffe
  trustSchema:
    identifier: urn:trust:helio-agent-governance-v1
    version: "1.0"
    governanceUri: https://policy.helio.example/governance
    verificationMethods: [spiffe, x509]
  attestations:
    - type: tag:agentrust.io,2026:trace-v0.1
      uri: https://trust.helio.example/runtime-evidence-profile.json
  provenance:
    - relation: publishedFrom
      sourceId: https://github.com/helio/agents
      sourceDigest: sha256:{{new string('d', 64)}}
  metadata:
    com.helio.verification:
      policyDigest: sha256:{{new string('c', 64)}}
      runtimeEvidenceExpected: true
  signatureIntent:
    algorithm: ES256
    keyRef: did:web:helio.example#catalog-signing
    required: {{bundleSignatureRequired.ToString().ToLowerInvariant()}}
""";

    private static string SourceTrustConfiguration(string identity, string identityType) => $$"""
schemaVersion: 1
source:
  identity: {{identity}}
  identityType: {{identityType}}
""";

    private sealed class TempDirectory : IDisposable
    {
        public string Path { get; } = System.IO.Path.Combine(System.IO.Path.GetTempPath(), "typeference-test-" + Guid.NewGuid().ToString("N"));
        public TempDirectory() => Directory.CreateDirectory(Path);
        public void Dispose() { if (Directory.Exists(Path)) Directory.Delete(Path, true); }
    }
}

internal static class ProcessStartInfoExtensions
{
    public static ProcessStartInfo WithArguments(this ProcessStartInfo info, IEnumerable<string> arguments)
    {
        foreach (var argument in arguments) info.ArgumentList.Add(argument);
        return info;
    }
}

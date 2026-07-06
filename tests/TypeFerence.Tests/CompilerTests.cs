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
    public void SystemObject_IsBehaviorFree()
    {
        var root = new TypeFerenceCompiler().Validate(Example).Single(x => x.Id == "system/object@1.0.0");
        Assert.True(root.Abstract);
        Assert.Empty(root.WorkingNorms);
        Assert.Empty(root.ContextFiles);
        Assert.Empty(root.Slots);
        Assert.Empty(root.Skills);
    }

    [Fact]
    public void ConcreteAgents_InheritThroughEnterpriseBase()
    {
        var agents = new TypeFerenceCompiler().Validate(Example).Where(x => !x.Abstract);
        Assert.All(agents, x => Assert.Equal("helio/enterprise-agent@1.0.0", x.Lineage[1]));
    }

    [Fact]
    public void Override_PreservesContractAndUsesDerivedDispatch()
    {
        var agent = new TypeFerenceCompiler().Validate(Example).Single(x => x.Id == "helio/payments-repo-agent@1.0.0");
        var skill = Assert.Single(agent.Skills);
        Assert.Equal("helio/skills/repository-status@1.0.0", skill.ContractId);
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
        Assert.Equal("80aad0f02eea35079fbab46cba445ab6adcd36f5a4f91b15cd514af4068355cc", TypeFerenceCompiler.HashDirectory(one.Path));
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
    public void InheritanceCycles_AreRejected()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["test/a@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "test/a@1.0.0", Extends = "test/b@1.0.0" },
            ["test/b@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "test/b@1.0.0", Extends = "test/a@1.0.0" }
        };
        Assert.Contains("cycle", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Provenance_CoversEffectiveScalarsInterfacesAndSkills()
    {
        var agent = new TypeFerenceCompiler().Validate(Example).Single(x => x.Id == "helio/executive-assistant@1.0.0");
        Assert.Contains(agent.Provenance, x => x.Field == "displayName" && x.Source == agent.Id);
        Assert.Contains(agent.Provenance, x => x.Field.StartsWith("interfaces.", StringComparison.Ordinal));
        Assert.All(agent.Skills, skill =>
        {
            Assert.Contains(skill.Provenance, x => x.Field == "skill.contract");
            Assert.Contains(skill.Provenance, x => x.Field == "skill.implementation");
        });
    }

    [Fact]
    public void InterfaceRequirements_AreEnforced()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["system/object@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "system/object@1.0.0", Abstract = true },
            ["test/contract@1.0.0"] = new() { SchemaVersion = 1, Kind = "interface", Id = "test/contract@1.0.0", RequiresSlots = ["owner"] },
            ["test/base@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "test/base@1.0.0", Abstract = true, Extends = "system/object@1.0.0", Implements = ["test/contract@1.0.0"] }
        };
        Assert.Contains("requires slot", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void MissingParentReferences_AreRejected()
    {
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["system/object@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "system/object@1.0.0", Abstract = true },
            ["test/base@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "test/base@1.0.0", Abstract = true, Extends = "test/missing@1.0.0" }
        };
        Assert.Contains("Agent not found", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void SkillOverrides_MustPreserveSchemas()
    {
        var baseSkill = new ResourceDocument
        {
            SchemaVersion = 1,
            Kind = "skill",
            Id = "test/skills/status@1.0.0",
            InputSchema = "{\"type\":\"object\",\"properties\":{\"focus\":{\"type\":\"string\"}}}",
            OutputSchema = "{\"type\":\"object\"}"
        };
        var replacement = new ResourceDocument
        {
            SchemaVersion = 1,
            Kind = "skill",
            Id = "test/skills/special-status@1.0.0",
            InputSchema = "{\"type\":\"object\",\"properties\":{\"count\":{\"type\":\"number\"}}}",
            OutputSchema = "{\"type\":\"object\"}"
        };
        var resources = new Dictionary<string, ResourceDocument>
        {
            ["system/object@1.0.0"] = new() { SchemaVersion = 1, Kind = "agent", Id = "system/object@1.0.0", Abstract = true },
            ["test/base@1.0.0"] = new()
            {
                SchemaVersion = 1,
                Kind = "agent",
                Id = "test/base@1.0.0",
                Abstract = true,
                Extends = "system/object@1.0.0",
                Skills = [new SkillBinding { Ref = baseSkill.Id }]
            },
            ["test/concrete@1.0.0"] = new()
            {
                SchemaVersion = 1,
                Kind = "agent",
                Id = "test/concrete@1.0.0",
                Extends = "test/base@1.0.0",
                Skills = [new SkillBinding { Ref = replacement.Id, Overrides = baseSkill.Id }]
            },
            [baseSkill.Id] = baseSkill,
            [replacement.Id] = replacement
        };
        Assert.Contains("changes the public contract", Assert.Throws<TypeFerenceException>(() => new TypeResolver(resources).ResolveAll()).Message, StringComparison.Ordinal);
    }

    [Fact]
    public void Interfaces_CannotInherit()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "root.yaml"), "schemaVersion: 1\nkind: agent\nid: system/object@1.0.0\nabstract: true\n");
        File.WriteAllText(Path.Combine(source.Path, "interface.yaml"), "schemaVersion: 1\nkind: interface\nid: test/contract@1.0.0\nextends: test/other@1.0.0\n");
        Assert.Contains("interfaces cannot extend", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("../outside.md", "escapes source root")]
    [InlineData("context/missing.md", "does not exist")]
    public void ContextPaths_MustStayInsideSourceAndExist(string path, string expected)
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "root.yaml"), $"schemaVersion: 1\nkind: agent\nid: system/object@1.0.0\nabstract: true\ncontextFiles:\n  - {path}\n");
        Assert.Contains(expected, Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void UnknownYamlFields_AreRejected()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "root.yaml"), """
schemaVersion: 1
kind: agent
id: system/object@1.0.0
abstract: true
unknownField: rejected
""");
        Assert.Contains("invalid YAML", Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(source.Path)).Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void ResourceIds_RequireSemanticVersions()
    {
        using var source = new TempDirectory();
        File.WriteAllText(Path.Combine(source.Path, "root.yaml"), """
schemaVersion: 1
kind: agent
id: system/object@latest
abstract: true
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

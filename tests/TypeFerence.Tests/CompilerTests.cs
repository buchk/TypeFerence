using System.Text.Json;
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

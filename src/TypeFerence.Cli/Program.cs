using System.Text.Json;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;
using ModelContextProtocol.Server;
using TypeFerence.Core;

return await Entry.RunAsync(args);

internal static class Entry
{
    public static async Task<int> RunAsync(string[] args)
    {
        try
        {
            if (args.Length == 0 || args[0] is "-h" or "--help" or "help") return Help();
            return args[0] switch
            {
                "validate" => Validate(args),
                "build" => Build(args),
                "inspect" => Inspect(args),
                "diff" => Diff(args),
                "serve" => await ServeAsync(args),
                _ => Fail($"Unknown command: {args[0]}")
            };
        }
        catch (TypeFerenceException ex) { return Fail(ex.Message); }
        catch (Exception ex) { return Fail(ex.ToString()); }
    }

    private static int Validate(string[] args)
    {
        var source = Required(args, 1, "source");
        var agents = new TypeFerenceCompiler().Validate(source);
        Console.WriteLine($"Valid: {agents.Count} agents resolved.");
        return 0;
    }

    private static int Build(string[] args)
    {
        var source = Required(args, 1, "source");
        var output = Option(args, "--out") ?? "dist";
        var target = Option(args, "--target") ?? "all";
        var publisherDomain = Option(args, "--publisher-domain");
        var emitArd = args.Contains("--emit-ard", StringComparer.Ordinal);
        if (emitArd && publisherDomain is null) throw new TypeFerenceException("--emit-ard requires --publisher-domain");
        if (!emitArd && publisherDomain is not null) throw new TypeFerenceException("--publisher-domain requires --emit-ard");
        var files = new TypeFerenceCompiler().Build(
            source,
            output,
            ParseTargets(target),
            emitArd ? new ArdPublicationOptions(publisherDomain!) : null);
        Console.WriteLine($"Built {files.Count} files at {Path.GetFullPath(output)}");
        Console.WriteLine($"SHA-256 {TypeFerenceCompiler.HashDirectory(output)}");
        return 0;
    }

    private static int Inspect(string[] args)
    {
        var source = Option(args, "--source") ?? ".";
        var id = Required(args, 1, "agent id");
        var agent = new TypeFerenceCompiler().Validate(source).SingleOrDefault(x => x.Id == id)
            ?? throw new TypeFerenceException($"Agent not found: {id}");
        Console.WriteLine(JsonSerializer.Serialize(agent, new JsonSerializerOptions { WriteIndented = true, PropertyNamingPolicy = JsonNamingPolicy.CamelCase }));
        return 0;
    }

    private static int Diff(string[] args)
    {
        var source = Required(args, 1, "source");
        var against = Option(args, "--against") ?? throw new TypeFerenceException("--against is required");
        var temp = Path.Combine(Path.GetTempPath(), "typeference-diff-" + Guid.NewGuid().ToString("N"));
        try
        {
            var publisherDomain = Option(args, "--publisher-domain");
            var emitArd = args.Contains("--emit-ard", StringComparer.Ordinal);
            if (emitArd && publisherDomain is null) throw new TypeFerenceException("--emit-ard requires --publisher-domain");
            if (!emitArd && publisherDomain is not null) throw new TypeFerenceException("--publisher-domain requires --emit-ard");
            new TypeFerenceCompiler().Build(
                source,
                temp,
                ParseTargets(Option(args, "--target") ?? "all"),
                emitArd ? new ArdPublicationOptions(publisherDomain!) : null);
            var result = DiffResult.Compare(against, temp);
            if (args.Contains("--json", StringComparer.Ordinal)) Console.WriteLine(JsonSerializer.Serialize(result, new JsonSerializerOptions { WriteIndented = true }));
            else
            {
                foreach (var x in result.Added) Console.WriteLine($"+ {x}");
                foreach (var x in result.Removed) Console.WriteLine($"- {x}");
                foreach (var x in result.Changed) Console.WriteLine($"~ {x}");
                if (!result.Different) Console.WriteLine("No differences.");
            }
            return result.Different ? 1 : 0;
        }
        finally { if (Directory.Exists(temp)) Directory.Delete(temp, true); }
    }

    private static async Task<int> ServeAsync(string[] args)
    {
        var source = Required(args, 1, "source or compiled directory");
        var agents = Directory.EnumerateFiles(source, "*.yaml", SearchOption.AllDirectories).Any()
            ? new TypeFerenceCompiler().Validate(source).Where(x => !x.Abstract).ToArray()
            : TypeFerenceCompiler.LoadCompiled(source).Where(x => !x.Abstract).ToArray();
        var tools = agents.SelectMany(agent => agent.Skills.Select(skill =>
        {
            Func<JsonElement, string> handler = arguments => JsonSerializer.Serialize(TypeFerenceCompiler.Invoke(agent, skill, arguments));
            return McpServerTool.Create(handler, new McpServerToolCreateOptions
            {
                Name = skill.DispatchName,
                Description = skill.Description,
                ReadOnly = true,
                Destructive = false,
                OpenWorld = false
            });
        })).ToArray();

        var builder = Host.CreateApplicationBuilder();
        builder.Logging.AddConsole(options => options.LogToStandardErrorThreshold = LogLevel.Trace);
        builder.Services.AddMcpServer().WithStdioServerTransport().WithTools(tools);
        await builder.Build().RunAsync();
        return 0;
    }

    private static CompilationTarget[] ParseTargets(string target) => target.ToLowerInvariant() switch
    {
        "all" => Enum.GetValues<CompilationTarget>(),
        "neutral" => [CompilationTarget.Neutral],
        "codex" => [CompilationTarget.Codex],
        "copilot" => [CompilationTarget.Copilot],
        "cursor" => [CompilationTarget.Cursor],
        _ => throw new TypeFerenceException($"Unknown target: {target}")
    };

    private static string Required(string[] args, int index, string name) => args.Length > index ? args[index] : throw new TypeFerenceException($"Missing {name}");
    private static string? Option(string[] args, string name) { var i = Array.IndexOf(args, name); return i >= 0 && i + 1 < args.Length ? args[i + 1] : null; }
    private static int Fail(string message) { Console.Error.WriteLine($"typeference: {message}"); return 2; }
    private static int Help()
    {
        Console.WriteLine("""
TypeFerence - typed coherence for AI agents

Commands:
  typeference validate <source>
  typeference build <source> [--target all|neutral|codex|copilot|cursor] [--out dist]
      [--emit-ard --publisher-domain example.com]
  typeference inspect <agent-id> [--source path]
  typeference diff <source> --against <compiled-dir> [--target all]
      [--emit-ard --publisher-domain example.com] [--json]
  typeference serve <source>
""");
        return 0;
    }
}

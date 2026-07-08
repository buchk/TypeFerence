using TypeFerence.Core;
using Xunit;

namespace TypeFerence.Tests;

/// <summary>
/// Targeted coverage for the ADR-0004 canonicalization rulings as implemented
/// in the reference implementation: canonical string ordering, the
/// separator-independent digest, and the ASCII key-space validations.
/// </summary>
public sealed class CanonicalizationTests
{
    [Fact]
    public void CanonicalOrder_SortsByCodePoint_NotUtf16CodeUnits()
    {
        // U+FFFD sorts below U+10000 in code point order, but ABOVE it in
        // UTF-16 code-unit order (surrogates start at 0xD800 < 0xFFFD).
        var supplementary = char.ConvertFromUtf32(0x10000);
        Assert.True(CanonicalOrder.Instance.Compare("�", supplementary) < 0);
        Assert.True(StringComparer.Ordinal.Compare("�", supplementary) > 0);

        // ASCII behavior matches ordinal exactly.
        Assert.True(CanonicalOrder.Instance.Compare("a", "b") < 0);
        Assert.True(CanonicalOrder.Instance.Compare("notes.md", "notes/x") < 0);
        Assert.True(CanonicalOrder.Instance.Compare("notes/x", "notes0.md") < 0);
        Assert.Equal(0, CanonicalOrder.Instance.Compare("same", "same"));
    }

    [Fact]
    public void HashDirectory_OrdersByRelativeForwardSlashPath()
    {
        // A tree whose names straddle the '/'..'\\' ASCII range hashed
        // per-file in the spec-defined order. If the implementation sorted
        // platform paths (backslashes on Windows), "notesA.md" would sort
        // before "notes/deep.md" here and the digest would differ per OS.
        using var temp = new TempDirectory();
        Directory.CreateDirectory(Path.Combine(temp.Path, "notes"));
        File.WriteAllText(Path.Combine(temp.Path, "notes", "deep.md"), "deep");
        File.WriteAllText(Path.Combine(temp.Path, "notesA.md"), "A");
        File.WriteAllText(Path.Combine(temp.Path, "notes.md"), "dot");

        var expectedPayload = "notes.md\0dot\0notes/deep.md\0deep\0notesA.md\0A\0";
        var expected = Convert.ToHexString(
                System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(expectedPayload)))
            .ToLowerInvariant();
        Assert.Equal(expected, TypeFerenceCompiler.HashDirectory(temp.Path));
    }

    [Fact]
    public void NonAsciiSlotNames_AreRejected()
    {
        using var temp = new TempDirectory();
        File.WriteAllText(Path.Combine(temp.Path, "policy.md"), "p");
        File.WriteAllText(Path.Combine(temp.Path, "agent.yaml"),
            """
            schemaVersion: 3
            kind: agent
            id: t/agent@1.0.0
            slots:
              règlePolitique: policy.md
            """);
        var ex = Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(temp.Path));
        Assert.Contains("ASCII identifier", ex.Message);
    }

    [Fact]
    public void NonAsciiRequiredSlotNames_AreRejected()
    {
        using var temp = new TempDirectory();
        File.WriteAllText(Path.Combine(temp.Path, "interface.yaml"),
            """
            schemaVersion: 3
            kind: interface
            id: t/interfaces/i@1.0.0
            requiresSlots:
              - política
            """);
        var ex = Assert.Throws<TypeFerenceException>(() => new ResourceLoader().Load(temp.Path));
        Assert.Contains("ASCII identifier", ex.Message);
    }

    [Fact]
    public void UppercaseYamlExtension_IsNotLoaded()
    {
        using var temp = new TempDirectory();
        File.WriteAllText(Path.Combine(temp.Path, "agent.yaml"),
            """
            schemaVersion: 3
            kind: agent
            id: t/agent@1.0.0
            """);
        // On a case-insensitive file system, EnumerateFiles("*.yaml") would
        // match this without the explicit ordinal extension pin.
        File.WriteAllText(Path.Combine(temp.Path, "SHOUTING.YAML"), "not a resource");
        var resources = new ResourceLoader().Load(temp.Path);
        Assert.Single(resources);
    }

    private sealed class TempDirectory : IDisposable
    {
        public string Path { get; } = Directory.CreateTempSubdirectory("typeference-test-").FullName;
        public void Dispose() => Directory.Delete(Path, true);
    }
}

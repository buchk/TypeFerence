using System.Text;

namespace TypeFerence.Core;

/// <summary>
/// Canonical string ordering per the specification: lexicographic by Unicode
/// code point (equivalently UTF-8 byte order). Differs from
/// <see cref="StringComparer.Ordinal"/> (UTF-16 code-unit order) for
/// supplementary-plane code points, which UTF-16 orders below U+E000.
/// </summary>
public sealed class CanonicalOrder : IComparer<string>
{
    public static readonly CanonicalOrder Instance = new();

    private CanonicalOrder()
    {
    }

    public int Compare(string? x, string? y)
    {
        if (ReferenceEquals(x, y)) return 0;
        if (x is null) return -1;
        if (y is null) return 1;
        var left = x.EnumerateRunes();
        var right = y.EnumerateRunes();
        while (true)
        {
            var hasLeft = left.MoveNext();
            var hasRight = right.MoveNext();
            if (!hasLeft || !hasRight) return hasLeft.CompareTo(hasRight);
            var compared = left.Current.Value.CompareTo(right.Current.Value);
            if (compared != 0) return compared;
        }
    }
}

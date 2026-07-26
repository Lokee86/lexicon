using Microsoft.CodeAnalysis;
using Microsoft.CodeAnalysis.CSharp;
using Microsoft.CodeAnalysis.Text;

internal sealed record SourceDocument(
    string AbsolutePath,
    byte[] Content,
    string RelativePath,
    SyntaxTree SyntaxTree);

internal sealed record RepositoryModel(
    CSharpCompilation Compilation,
    IReadOnlyList<SourceDocument> Documents,
    string Root);

internal static class Discovery
{
    private static readonly HashSet<string> IgnoredDirectories = new(StringComparer.OrdinalIgnoreCase)
    {
        ".git", ".lexicon", ".worktrees", ".workingtrees", ".vs", ".idea",
        "bin", "obj", "packages", "node_modules", "vendor", "generated",
        "artifacts", "build", "dist", "target", "coverage", "tmp", "temp",
    };

    internal static RepositoryModel Load(string repositoryRoot)
    {
        var root = Path.GetFullPath(repositoryRoot);
        var documents = DiscoverFiles(root)
            .Select(path => LoadDocument(root, path))
            .OrderBy(document => document.RelativePath, StringComparer.Ordinal)
            .ToArray();
        var compilation = CSharpCompilation.Create(
            assemblyName: "Lexicon.CSharp.Analysis",
            syntaxTrees: documents.Select(document => document.SyntaxTree),
            references: TrustedPlatformReferences(),
            options: new CSharpCompilationOptions(
                OutputKind.DynamicallyLinkedLibrary,
                allowUnsafe: true,
                nullableContextOptions: NullableContextOptions.Enable));
        return new RepositoryModel(compilation, documents, root);
    }

    private static IEnumerable<string> DiscoverFiles(string root)
    {
        var pending = new Stack<string>();
        pending.Push(root);
        while (pending.Count > 0)
        {
            var directory = pending.Pop();
            foreach (var child in Directory.EnumerateDirectories(directory)
                         .OrderByDescending(path => path, StringComparer.Ordinal))
            {
                if (!IgnoredDirectories.Contains(Path.GetFileName(child)))
                {
                    pending.Push(child);
                }
            }

            foreach (var file in Directory.EnumerateFiles(directory, "*.cs")
                         .OrderBy(path => path, StringComparer.Ordinal))
            {
                yield return file;
            }
        }
    }

    private static SourceDocument LoadDocument(string root, string absolutePath)
    {
        var content = File.ReadAllBytes(absolutePath);
        var relativePath = Facts.NormalizePath(Path.GetRelativePath(root, absolutePath));
        var text = SourceText.From(content, content.Length, System.Text.Encoding.UTF8, canBeEmbedded: false);
        var tree = CSharpSyntaxTree.ParseText(
            text,
            new CSharpParseOptions(LanguageVersion.Preview, DocumentationMode.Parse),
            relativePath);
        return new SourceDocument(absolutePath, content, relativePath, tree);
    }

    private static IReadOnlyList<MetadataReference> TrustedPlatformReferences()
    {
        var value = AppContext.GetData("TRUSTED_PLATFORM_ASSEMBLIES") as string;
        if (string.IsNullOrWhiteSpace(value))
        {
            throw new InvalidOperationException("TRUSTED_PLATFORM_ASSEMBLIES is unavailable");
        }

        return value.Split(Path.PathSeparator, StringSplitOptions.RemoveEmptyEntries)
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .OrderBy(path => path, StringComparer.OrdinalIgnoreCase)
            .Select(path => MetadataReference.CreateFromFile(path))
            .ToArray();
    }
}

using Microsoft.CodeAnalysis;

internal sealed partial class Analysis
{
    private readonly FactStore facts = new();
    private readonly Dictionary<SyntaxTree, string> fileIds = new();
    private readonly RepositoryModel model;
    private readonly string repositoryId;

    private Analysis(RepositoryModel model, string repositoryName)
    {
        this.model = model;
        repositoryId = facts.AddNode(
            "repository",
            repositoryName,
            ".",
            repositoryName,
            identity: repositoryName);
    }

    internal static FactStore Run(string repositoryRoot, string repositoryName)
    {
        var analysis = new Analysis(Discovery.Load(repositoryRoot), repositoryName);
        analysis.EmitFiles();
        analysis.EmitDeclarations();
        analysis.EmitRelationships();
        analysis.EmitCalls();
        analysis.EmitDataflow();
        return analysis.facts;
    }

    private void EmitFiles()
    {
        foreach (var document in model.Documents)
        {
            var syntaxRoot = document.SyntaxTree.GetRoot();
            var fileId = facts.AddNode(
                "file",
                Path.GetFileName(document.RelativePath),
                document.RelativePath,
                document.RelativePath,
                identity: document.RelativePath,
                span: Span(document.RelativePath, syntaxRoot.GetLocation()),
                contentId: Facts.ContentId(document.Content),
                owner: document.RelativePath);
            fileIds[document.SyntaxTree] = fileId;
            facts.AddEdge(new EdgeRecord
            {
                Owner = document.RelativePath,
                Record = "edge",
                Relation = "contains",
                Source = repositoryId,
                Target = fileId,
            });
        }
    }

    internal static SpanRecord Span(string path, Location location)
    {
        var lineSpan = location.GetLineSpan();
        return new SpanRecord
        {
            EndColumn = lineSpan.EndLinePosition.Character + 1,
            EndLine = lineSpan.EndLinePosition.Line + 1,
            Path = path,
            StartColumn = lineSpan.StartLinePosition.Character + 1,
            StartLine = lineSpan.StartLinePosition.Line + 1,
        };
    }

    private string PathFor(SyntaxTree tree)
    {
        return Facts.NormalizePath(tree.FilePath);
    }

    private void AddEdge(string source, string target, string relation, string owner, Location? location = null)
    {
        facts.AddEdge(new EdgeRecord
        {
            Owner = owner,
            Record = "edge",
            Relation = relation,
            Source = source,
            Span = location is null ? null : Span(owner, location),
            Target = target,
        });
    }
}

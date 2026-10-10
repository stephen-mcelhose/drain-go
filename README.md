# drain-go

A dependency-free Go implementation of **Drain**, the fixed-depth online log-template mining algorithm described in:

> Pinjia He, Jieming Zhu, Zibin Zheng, and Michael R. Lyu. *Drain: An Online Log Parsing Approach with Fixed Depth Tree*. ICWS 2017.

The parser is designed for **long-running log streams**:

- `Train` classifies an incoming line and learns a new or generalized template.
- `Match` classifies against learned templates only; it never changes the model.
- A fixed-depth tree narrows candidate clusters by token count and leading tokens.
- `MaxChildren` prevents branch explosion, while digit-containing tokens can route through a wildcard branch.
- Optional ordered masks replace known dynamic values (such as IP addresses) before parsing.
- `MaxClusters` bounds memory with least-recently-used cluster eviction.
- `Serialize` and `Load` persist the full parser state so long-running streams survive restarts.
- The exported `Parser` API is safe for concurrent use.

## Install

```bash
go get github.com/stephen-mcelhose/drain-go
```

For local development:

```bash
go test -race ./...
```

## Streaming usage

```go
package main

import (
    "fmt"
    "log"

    drain "github.com/stephen-mcelhose/drain-go"
)

func main() {
    parser, err := drain.New(drain.Config{
        Depth:               4,
        SimilarityThreshold: 0.4,
        MaxClusters:         10_000, // 0 means unbounded
        Masks: []drain.Mask{
            {Pattern: `\b(?:\d{1,3}\.){3}\d{1,3}\b`, Name: "IP"},
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    for _, line := range []string{
        "connected to 10.0.0.1",
        "connected to 192.168.0.1",
        "user alice logged in",
        "user bob logged in",
    } {
        result := parser.Train(line)
        fmt.Printf("id=%d change=%s template=%q\n", result.ClusterID, result.Change, result.Template)
    }

    // Inference-only: a non-match does not create a cluster.
    if result, ok := parser.Match("user carol logged in"); ok {
        fmt.Printf("classified as template %d: %s\n", result.ClusterID, result.Template)
    }
}
```

Expected learned templates include:

```text
connected to <IP>
user <*> logged in
```

## Persistence

`Serialize` encodes the full parser state — clusters, tree, LRU order, and configuration — as a versioned JSON byte slice. `Load` restores it exactly, including eviction order, so a restarted process picks up where it left off.

```go
// Save.
data, err := parser.Serialize()
if err != nil {
    log.Fatal(err)
}
if err := os.WriteFile("parser.json", data, 0600); err != nil {
    log.Fatal(err)
}

// Restore.
data, err = os.ReadFile("parser.json")
if err != nil {
    log.Fatal(err)
}
parser, err = drain.Load(data)
if err != nil {
    log.Fatal(err)
}
```

## Match options

`Match` accepts functional options to adjust search behaviour per call:

```go
// Soft inference: accept lines that match ≥ 70 % of template tokens.
r, ok := parser.Match(line, drain.WithSimilarityThreshold(0.7))

// Fastest routing: tree only, no linear-scan fallback.
r, ok = parser.Match(line, drain.WithStrategy(drain.TreeOnly))

// Most accurate: always scan all same-length clusters.
r, ok = parser.Match(line, drain.WithStrategy(drain.FullSearchAlways))
```

## Configuration

| Field                      | Default    | Purpose                                                                                                                                      |
|----------------------------|:----------:|----------------------------------------------------------------------------------------------------------------------------------------------|
| `Depth`                    | `4`        | Total tree depth; at least `3`. A value of 4 uses up to two leading tokens for routing.                                                      |
| `SimilarityThreshold`      | `0.4`      | Minimum fraction of equal non-wildcard tokens for an existing cluster to accept a training message.                                          |
| `MaxChildren`              | `100`      | Maximum token children per internal tree node.                                                                                               |
| `MaxClusters`              | unbounded  | Number of live clusters; at capacity, the least recently used cluster is evicted.                                                            |
| `ExtraDelimiters`          | none       | Additional strings treated as token separators.                                                                                              |
| `ParametrizeNumericTokens` | `true`     | Route tokens containing digits through the wildcard tree branch. This is a `*bool`; pass a pointer to `false` to disable it explicitly.      |
| `Masks`                    | none       | Ordered Go RE2 regular expressions replaced by `<Name>` before tokenization.                                                                 |

## Algorithm behavior

Drain first tokenizes a preprocessed log message. It reaches a candidate leaf through a tree whose first level is the token count and whose remaining fixed levels use leading tokens. Dynamic or highly variable routing positions use `<*>`. Each leaf stores candidate clusters. For each candidate, similarity is the number of equal non-wildcard tokens divided by message length. The best candidate at or above the threshold is selected; otherwise a new cluster is added. When an existing cluster accepts a line, every differing template position becomes `<*>`.

`Match` requires a perfect template match, counting `<*>` as matching a token. Its default `FullSearchFallback` strategy protects inference from a tree-routing miss by scanning only clusters of the same token count. Use `TreeOnly` for fastest routing, or `FullSearchAlways` when choosing the optimal same-length candidate is more important than performance. Both the strategy and the similarity threshold are configurable per call via `MatchOption`.

## Notes

- Feed the free-text message body rather than timestamp, host, and severity fields when possible.
- This package intentionally focuses on streaming mining and inference. It does not include persistence adapters or a log-format parser.
- Mask expressions use Go's RE2 syntax, which does not support look-around assertions.

## License

MIT — see [LICENSE](LICENSE).

## References

- He et al., [Drain paper (PDF)](https://jiemingzhu.github.io/pub/pjhe_icws2017.pdf)
- [LogPAI Drain3](https://github.com/logpai/Drain3) (MIT), consulted for production-oriented extensions such as masking, inference, and bounded cluster storage.
- See [ATTRIBUTION.md](ATTRIBUTION.md) for full citation details.

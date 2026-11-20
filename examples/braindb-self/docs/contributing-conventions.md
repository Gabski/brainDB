# Conventions in this repository

- **No emoji, anywhere** (Addendum N). Icons are inline SVG. `TestNoEmoji` fails the build on a stray glyph; it has
  already caught the navigation glyph in `docs/index.html`.
- **gofmt is a gate, not a suggestion.** CI runs `gofmt -l` and fails on any output, then `go vet`,
  `go test -race`, `go build`, then the web job: `npm ci`, typecheck, unit tests, the emoji lint, the Vite build and
  the Storybook build. `make test` mirrors the same checks locally.
- **Pure modules get Vitest tests, components get Storybook stories.** 119 tests over `src/model`, 75% statement
  coverage. The suite earns its place: it caught a filter that ORed a typed `kind:` prefix with a kind chip instead
  of ANDing them.
- **Server JSON is typed as it is sent.** Three bugs in 0.0.2 were the same shape: a Go nil slice marshals to JSON
  `null`, and the UI typed the field as a list. Send `[]`, type it nullable, and keep a fixture per backend.
- **Every example must load in `go test`.** Adding an example means adding its name to the lists in
  `internal/brain/brain_test.go` and `internal/brain/importer_test.go`.
- **Documentation parity.** `README.md` and `docs/index.html` describe the same system; a change to one is a change
  to both. `docs/SPEC.md` is the contract, and the addenda are referenced by letter.
- **Numbers carry their provenance.** Engine, dimension, node count, k and date, or it is not a measurement.
  Unmeasured figures are labelled as targets.

# Releasing

`termnav` is the navigation / list-windowing engine in a three-module shared-UI
stack, consumed by [passage](https://github.com/0xbenc/passage) and
[ssherpa](https://github.com/0xbenc/ssherpa):

```
termtheme (leaf; no bubbletea)
   ├─► termnav     (this repo — depends on termtheme + bubbletea)
   ├─► termchrome  (box/footer/kvrow + glyphs/countdown; termtheme only)
   └─► consumed by ─► passage, ssherpa
```

Tag **bottom-up**: `termtheme` → `{termnav, termchrome}` → `{passage, ssherpa}`.
If a change also needs a new termtheme, tag termtheme **first**, then pin it here
before tagging termnav.

## Release order (when a change touches termnav)

1. **Dev loop:** in each consumer add `replace github.com/0xbenc/termnav => ../termnav`,
   apply call-site changes, and get both green (`go build ./... && go test ./...`).
2. **Tag termnav** (after both consumers are green locally):

   ```sh
   git push origin main
   git tag -a vX.Y.Z -m "termnav vX.Y.Z — ..."
   git push origin vX.Y.Z
   ```

3. **Pin in each consumer** (one commit each):

   ```sh
   go get github.com/0xbenc/termnav@vX.Y.Z
   go mod edit -dropreplace=github.com/0xbenc/termnav
   go mod tidy && go test ./...
   git commit -am "<app>: pin termnav vX.Y.Z (drop local replace)"
   ```

**No `replace` in any released `go.mod`** (`go mod verify` / goreleaser `go mod verify`
enforces it). **Pin lockstep:** passage and ssherpa end on identical
termtheme/termnav/termchrome versions (hotfix exception: one app may bump ahead
urgently; restore lockstep next release).

## Versioning

Semantic versioning. A change to an exported signature or to navigation/windowing
behavior is at least a **minor** bump; breaking one is a **major** bump.

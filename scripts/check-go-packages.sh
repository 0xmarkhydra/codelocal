#!/usr/bin/env bash
set -u
rm -rf .go-check
mkdir -p .go-check
status=0
while IFS= read -r pkg; do
  name="${pkg#github.com/0xmarkhydra/codelocal/}"
  name="${name//\//__}"
  [ "$name" = "github.com__0xmarkhydra__codelocal" ] && name="root"
  if go test "$pkg" > ".go-check/${name}.log" 2>&1; then
    mv ".go-check/${name}.log" ".go-check/${name}.pass"
  else
    mv ".go-check/${name}.log" ".go-check/${name}.fail"
    status=1
  fi
done < <(go list ./...)
if [ "$status" -eq 0 ]; then
  printf 'PASS\n' > .go-check/ALL_PASS
fi
exit "$status"

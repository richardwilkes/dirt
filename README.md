# dirt
Runs linting checks against Go code in a repository. Two groups of linters are
executed, a "fast" group and a "slow" group. The fast group consists of gofmt,
goimports, golint, ineffassign, misspell, vet. The slow group consists of
megacheck, errchk, interfacer, unconvert, structcheck.

The intent is for this to incorporate a way of ensuring only one set of
linting is being performed at once, since IDEs like vscode can easily swamp
the machine making it unresponsive if you have them set to run the linters on
save and you save many files in succession. This capability has not yet been
implemented, but will be worked on next.
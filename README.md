# dirt
Runs linting checks against Go code in a repository. Two groups of linters are
executed, a "fast" group and a "slow" group. The fast group consists of gofmt,
goimports, golint, ineffassign, misspell, vet. The slow group consists of
staticcheck, errcheck, unconvert.

To run this from vscode, add these lines to preferences:

    {
        "go.lintTool": "dirt",
        "go.lintFlags": ["-1"],
        "go.lintOnSave": "workspace",
        "go.vetOnSave": "off",
    }

# gh-private-fork

A GitHub CLI extension that adds a `private-fork` subcommand to `gh`, allowing you to create private forks of public repositories.

## Installation

Make sure you have [GitHub CLI (gh)](https://github.com/cli/cli#installation) installed.

```bash
# install
gh ext install admin-aftermath/gh-private-fork
# upgrade
gh ext upgrade admin-aftermath/gh-private-fork
# uninstall
gh ext remove admin-aftermath/gh-private-fork
```

## Usage

```bash
# Create a private fork of a repository
gh private-fork some/repo

# Create a private fork in an organization
gh private-fork some/repo --org myorg

# Create a private fork in an organization, rename the fork, and clone it
gh private-fork some/repo --org myorg --fork-name new-name --clone

# Fork current repository privately
gh private-fork
```

## Development

```bash
gh extension install .

go build && gh private-fork ...
```

# Release Documentation

This document contains instructions to help maintainers ensure consistent releases.

## Release Instructions

### Select the commit that will become the release

```code
$ git checkout master
```

### Check that desired changes are committed

Use `git log` to check that all changes have been commited.

### Tag and push

```code
$ make update-tag VERSION=x.y.z
```

The above target creates and pushes a tag for each plugin.

Run `make print-version` to double check that the build system will correctly pick up the tag
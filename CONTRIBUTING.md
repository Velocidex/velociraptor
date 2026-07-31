# Contributing to the Velociraptor project

Thank you for your interest in contributing to the Velociraptor
project! Velociraptor is an open source community-driven project and
we welcome contributions from the community.

There are many ways you can contribute: reporting bugs, improving the
documentation, submitting artifacts, writing code, improving the GUI,
and more. This guide explains the project structure and where the most
common types of contributions are made.

## Table of Contents

- [Project Structure](#project-structure)
- [Contributions to the main repo](#contributions-to-the-main-repo)
- [Reporting Bugs and Security Issues](#reporting-bugs-and-security-issues)
- [Contributions to other project repos](#contributions-to-other-project-repos)
- [Getting Help](#getting-help)
- [License](#license)

## Project Structure

Before you contribute it might be useful to understand the overall
project structure.

This is the main repository for Velociraptor, however the project is
structured into several repos to make things more manageable. For
example, the [VQL language core](https://github.com/Velocidex/vfilter),
[large or complex plugins](https://github.com/Velocidex/vtypes), and
the [documentation website](https://github.com/Velocidex/velociraptor-docs)
are maintained in their own repos which you can browse at
[https://github.com/orgs/Velocidex/repositories](https://github.com/orgs/Velocidex/repositories).

For all Velocidex repositories we use the standard GitHub
[fork-and-PR](https://docs.github.com/en/get-started/quickstart/contributing-to-projects)
("triangular") workflow.

We require all contributors to sign a
[Contributor License Agreement (CLA)](https://github.com/Velocidex/velociraptor/blob/master/CLA.md)
before we can accept their contributions. See the
[License](#license) section for details.

## Contributions to the main repo

### Code contributions

Velociraptor is written in [Go](https://golang.org/) and the bulk of
the code lives in this repository. If you are planning to contribute
code, please:

- Set up your development environment by following the
  [Building from source](https://github.com/Velocidex/velociraptor#building-from-source)
  section of the README.
- Keep changes focused and well tested. Tests can be run with
  `make test`.
- Follow standard Go conventions. The repository contains a
  [staticcheck.conf](https://github.com/Velocidex/velociraptor/blob/master/staticcheck.conf)
  with the project's lint settings.

The code is organized into a number of top-level packages: for
example, the VQL engine lives under `vql/`, server side services under
`services/`, filesystem accessors under `accessors/`, and more. When
in doubt about where your change belongs, ask us on
[Discord](https://docs.velociraptor.app/discord/) before writing code.
It's always a good idea to discuss your ideas or proposed changes
before investing your time into them, and the discussions themselves
often have as much value as the potential changes. Of course if it's
just a simple change then go right ahead and submit a PR, and any
discussions around the change can also be discussed there.

In addition to Go code, Velociraptor also includes some scripts (for
example, for installer packaging) that can always use additional
refinement. So if you have expertise in those particular scripting
areas, your contribution may be valuable even if you have zero Go
knowledge.

### Artifacts

Artifacts in the main repo are bundled into the Velociraptor binary
and generally require
[tests](https://github.com/Velocidex/velociraptor/blob/master/artifacts/testdata/server/testcases/README.md)
which ensure that Velociraptor does not ship with non-working
artifacts.

In this sense, bundled artifacts are held to a higher quality standard
than artifacts contributed to the community artifact exchange, as
well as meeting other requirements such as being broadly
useful rather than being for niche use cases. The
[Contributor Guidelines](https://github.com/Velocidex/velociraptor-docs/blob/master/CONTRIBUTING.md#where-should-i-contribute-my-artifact)
for the artifact exchange outline some criteria to help you decide
where your artifact contribution would be most appropriate.

The CI build pipelines run these golden tests automatically, and the
built-in artifact verifier also runs against all bundled artifacts.

### GUI (React)

The Velociraptor GUI is a React application with its code located under
[`gui/velociraptor/`](https://github.com/Velocidex/velociraptor/tree/master/gui/velociraptor).
See the
[GUI development README](https://github.com/Velocidex/velociraptor/blob/master/gui/velociraptor/README.md)
for details on setting up a development server.

#### Themes

The GUI supports user-selectable themes. A theme is a CSS file in
[`gui/velociraptor/src/themes/`](https://github.com/Velocidex/velociraptor/tree/master/gui/velociraptor/src/themes).
Contributing a new theme involves:

1. Adding a new `<name>.css` file to `src/themes/`.
2. Importing it in
   [`src/App.jsx`](https://github.com/Velocidex/velociraptor/blob/master/gui/velociraptor/src/App.jsx).
3. Adding the theme to the user settings dropdown in
   `src/components/core/user.jsx`.
4. Mapping the theme to an ACE editor theme (for the VQL editor) if
   appropriate.
5. Adding the theme name to the server side validation whitelist in
   [services/users/validation.go](https://github.com/Velocidex/velociraptor/blob/master/services/users/validation.go).

The
[VS Code Dark Modern theme PR](https://github.com/Velocidex/velociraptor/pull/4568)
is a good example of a complete theme contribution.

#### Language Translations

The Velociraptor GUI is translated into several languages, and most of
the widely-spoken languages are already covered (The GUI ships with
German, Spanish, French, Japanese, Portuguese, and Vietnamese
translations). The preferred language is set as a
[user preference](https://docs.velociraptor.app/docs/gui/user_preferences/).

Translations are maintained under
[`gui/velociraptor/src/components/i8n/`](https://github.com/Velocidex/velociraptor/tree/master/gui/velociraptor/src/components/i8n)
and maintenance is done with the help of a script — see the
[i8n README](https://github.com/Velocidex/velociraptor/blob/master/gui/velociraptor/src/components/i8n/README.md)
for the full workflow.

There are two ways to contribute to translations:

##### Adding a new language

If your preferred language isn't included then you can help
your fellow speakers by adding it. For example, Chinese and Russian
are not represented even though we are quite sure that we
have many users who are native speakers of those languages.

To add a new language, create a `<lang>.jsx` file and register it in
[`i8n.jsx`](https://github.com/Velocidex/velociraptor/blob/master/gui/velociraptor/src/components/i8n/i8n.jsx).
This
[Vietnamese translation PR](https://github.com/Velocidex/velociraptor/pull/2680)
is a good example of a complete new-language contribution.

##### Reviewing and improving existing translations

`make translations` runs
[`scripts/find_i8n_translations.py`](https://github.com/Velocidex/velociraptor/blob/master/scripts/find_i8n_translations.py),
which audits the source code for strings that are missing from each
language's translation files. This workflow is primarily for
maintaining existing translations — for example when new GUI
components are added or the wording of an existing element changes.
The scripts can also be used when setting up a new language, but
that is not their main purpose.

Native speakers can help by reviewing and improving translations:
machine translations may contain awkward or incorrect renderings of
IT-related jargon, so we encourage fluent speakers to review the
automated entries and promote good translations into the curated
`<lang>.jsx` files.

## Reporting Bugs and Security Issues

Bug reports are greatly appreciated contributions. If you find a bug:

- Search the existing
  [issues](https://github.com/Velocidex/velociraptor/issues) first to
  avoid duplicates.
- File a new issue including details such as the Velociraptor version,
  your platform, and steps to reproduce the problem.

If you have found a security vulnerability, **do not** open a
public issue. Follow the disclosure policy in
[security.md](https://github.com/Velocidex/velociraptor/blob/master/security.md)
and contact security@rapid7.com.

## Contributions to other project repos

Most contribution activity happens in the main Velociraptor
repository, but the project is spread across a number of repositories.
The following table lists the major ones; for the full list see the
[Velocidex GitHub organization](https://github.com/orgs/Velocidex/repositories).

| Repository | Purpose |
| --- | --- |
| [velociraptor](https://github.com/Velocidex/velociraptor) (this repo) | Main server, client, GUI and built-in artifacts |
| [velociraptor-docs](https://github.com/Velocidex/velociraptor-docs) | Documentation website and Artifact Exchange |
| [vfilter](https://github.com/Velocidex/vfilter) | The VQL language core |

Most artifact contributions happen through the documentation
repository and the specialized artifact projects described later.

### Documentation Contributions

The [Velociraptor documentation website](https://docs.velociraptor.app)
and the community [Artifact Exchange](https://docs.velociraptor.app/exchange/)
are maintained in the
[velociraptor-docs](https://github.com/Velocidex/velociraptor-docs)
repository. See that repository's
[CONTRIBUTING.md](https://github.com/Velocidex/velociraptor-docs/blob/master/CONTRIBUTING.md)
for the full details. In brief:

The [Documentation Development Guidelines](https://docs.velociraptor.app/dev/)
describe how to set up a local Hugo preview server and provide
style guidelines, image guidelines, and information about
prose linting.

### Contributing Artifacts to the Artifact Exchange

Velociraptor's VQL language is designed to lower the bar for
contributions, and the Artifact Exchange is the place to share your
artifacts with the community. Before contributing, read the
[Where should I contribute my artifact?](https://github.com/Velocidex/velociraptor-docs/blob/master/CONTRIBUTING.md#where-should-i-contribute-my-artifact)
section of the docs repository's CONTRIBUTING guide, which explains
the quality bar for built-in artifacts vs Exchange artifacts.

[Chat with us on Discord](https://docs.velociraptor.app/discord/)
before contributing if you are unsure where your artifact belongs —
we can help you avoid frustration or wasted effort.

Artifact Exchange contributions consist of adding a YAML artifact file
under `content/exchange/artifacts/` in the `velociraptor-docs`
repository. You can create a fork and submit a Pull Request entirely
from the GitHub website — no local tooling required.

### Contributing to specialized artifact projects

Some Velociraptor artifacts cover large knowledge domains and are
therefore maintained in separate repos. These artifacts are generally
quite large and are optionally imported into Velociraptor after
installation rather than being bundled into the binary.

The goal of these artifacts is to support a broad set of use cases
that have a common goal or common collection logic, rather than
having many small artifacts that duplicate the same collection logic.

- **Triage artifacts** (search for files, collect, hash, enrich): use
  the [Triage Artifact project](https://github.com/Velocidex/velociraptor-triage-collector)
  instead. See the [Triage Artifacts site](https://triage.velocidex.com/)
  for details.

- **Registry parsing artifacts** (search registry keys, parse binary
  values): use the [Registry Hunter](https://registry-hunter.velocidex.com/)
  project instead.

- **Artifacts targeting web browsers and OS database files** (SQLite,
  LevelDB, ESE): use the
  [SQLiteHunter](https://github.com/Velocidex/SQLiteHunter) project
  instead.

- **Sigma rule-based detections**: contribute to our
  [Sigma Rules](https://github.com/Velocidex/velociraptor-sigma-rules)
  project instead.

## Getting Help

If you need help or want to discuss an idea before contributing:

- Chat with us on
  [Discord](https://docs.velociraptor.app/discord/).
- Ask questions on the
  [velociraptor-discuss mailing list](https://groups.google.com/g/velociraptor-discuss)
  (velociraptor-discuss@googlegroups.com).

---

## License

The main Velociraptor repository is licensed under the
[GNU Affero General Public License v3.0](https://www.gnu.org/licenses/agpl-3.0.html).
By contributing to this repository you agree that your contributions
will be licensed under the AGPLv3. External contributors are also
required to sign the
[Contributor License Agreement](https://github.com/Velocidex/velociraptor/blob/master/CLA.md).

Note that other repositories in the organization may use different
licenses — for example, the documentation and Artifact Exchange
content in the velociraptor-docs repository is licensed under the
[Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International License](http://creativecommons.org/licenses/by-nc-sa/4.0/).

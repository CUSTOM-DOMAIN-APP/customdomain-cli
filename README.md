<p align="center"><picture><source media="(prefers-color-scheme: dark)" srcset=".github/media/customdomain-mark-dark.png"><img src=".github/media/customdomain-mark.png" alt="CustomDomain" width="56"></picture></p>
<h1 align="center">CustomDomain CLI</h1>
<p align="center"><b>Sign in from your terminal, give your project its API key, and check what stands between a domain and your app.</b></p>
<p align="center"><a href="https://www.npmjs.com/package/@customdomain/cli"><img src="https://img.shields.io/npm/v/@customdomain/cli?style=flat&color=1c1917&logo=npm&logoColor=white&label=%40customdomain%2Fcli" alt="npm"></a> <a href="https://github.com/CUSTOM-DOMAIN-APP/homebrew-tap"><img src="https://img.shields.io/badge/Homebrew-custom--domain--app%2Ftap-1c1917?style=flat&logo=homebrew&logoColor=white" alt="Homebrew"></a> <a href="https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/releases"><img src="https://img.shields.io/github/v/release/CUSTOM-DOMAIN-APP/customdomain-cli?style=flat&color=1c1917&logo=github&logoColor=white&label=release" alt="release"></a> <a href="https://docs.customdomain.ai/docs"><img src="https://img.shields.io/badge/docs-docs.customdomain.ai-1c1917?style=flat" alt="docs"></a> <a href="https://www.producthunt.com/products/custom-domain"><img src="https://img.shields.io/badge/Product%20Hunt-CustomDomain-1c1917?style=flat&logo=producthunt&logoColor=white" alt="Product Hunt"></a></p>
<p align="center"><picture><source media="(prefers-color-scheme: dark)" srcset=".github/media/widget-flow-dark.png"><img src=".github/media/widget-flow-light.png" alt="The CustomDomain widget: your user types their domain, signs in at their provider, and it goes live" width="860"></picture></p>

## Install

|  | Install |
|---|---|
| <picture><source media="(prefers-color-scheme: dark)" srcset="https://cdn.simpleicons.org/npm/e7e5e4"><img src="https://cdn.simpleicons.org/npm/1c1917" alt="npm" width="20" height="20"></picture> | `npm install -g @customdomain/cli` |
| <picture><source media="(prefers-color-scheme: dark)" srcset="https://cdn.simpleicons.org/homebrew/e7e5e4"><img src="https://cdn.simpleicons.org/homebrew/1c1917" alt="Homebrew" width="20" height="20"></picture> | `brew install custom-domain-app/tap/customdomain` |
| <picture><source media="(prefers-color-scheme: dark)" srcset="https://cdn.simpleicons.org/github/e7e5e4"><img src="https://cdn.simpleicons.org/github/1c1917" alt="GitHub" width="20" height="20"></picture> | Binaries for macOS, Linux and Windows on the [releases page](https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/releases) |

Or run it once without installing: `npx @customdomain/cli init`.

## Use

```text
customdomain init [--app app_123] [--env-file .env.local]    create an API key and save it to the project's env file
customdomain login                                           sign in from this terminal
customdomain logout                                          end this terminal's access
customdomain whoami                                          who this terminal acts as
customdomain domains                                         list your connected domains
customdomain doctor <domain>                                 where a domain's DNS lives and what is in the way
customdomain version                                         print the version
```

`init` is the quickest start. It opens a page where you approve the sign-in, creates an API key for your application
and writes `CUSTOMDOMAIN_API_KEY` and `CUSTOMDOMAIN_APPLICATION_ID` to your env file, so the key never has to be
copied or pasted. It works the same when a coding agent runs it for you.

Docs: [docs.customdomain.ai](https://docs.customdomain.ai).

## License

Apache-2.0. This repository is published from the CustomDomain™ source; open issues here.

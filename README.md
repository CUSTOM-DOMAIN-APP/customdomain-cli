# customdomain

The command line client for [CustomDomain™](https://customdomain.ai): sign in from your terminal, give a project its
API key, and check what stands between a domain and your app.

## Install

```bash
npm install -g customdomain
```

```bash
brew install custom-domain-app/tap/customdomain
```

Or run it once without installing: `npx customdomain init`. Binaries for macOS, Linux and Windows are also on the
[releases page](https://github.com/CUSTOM-DOMAIN-APP/customdomain-cli/releases).

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

# Component CI

The GitHub Actions workflow is `.github/workflows/ci.yml`. It runs for pull requests and pushes to development branches. Go checks use Go 1.26.3; Python checks use Python 3.11.

## Run locally

`GOWORK=off go build ./...` and `GOWORK=off go test ./...`.

PostgreSQL integration requires a separate configured database.

## Evidence and branch rules

Inspect the Actions job log for setup and failure details. Test jobs upload available logs or JUnit reports for seven days. A setup failure may happen before the report exists. Configure private sibling repository access using the Actions secret `CI_REPOSITORY_READ_TOKEN` when the workflow performs cross-repository checkout. After a successful run on the target branch, an administrator may require the applicable job under the repository branch rules; a workflow file alone does not enforce merges.

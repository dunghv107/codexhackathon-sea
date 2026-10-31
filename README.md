# Development environment

The root [compose.yaml](compose.yaml) runs frontend, backend, KBS, PostgreSQL,
Redis and migrations. See [container setup and CI checks](docs/CONTAINERS.md)
for credentials, runtime/debug variants, logs, and verification commands.

See [self-hosted CI and temporary SonarQube setup](docs/PIPELINE.md) for the
GitHub Actions pipeline and an external temporary SonarQube server.

See [phone identity, Network and simulated call setup](docs/PHONE_NETWORK_CALL.md)
for migration 6, required phone HMAC configuration and the local PostgreSQL smoke test.

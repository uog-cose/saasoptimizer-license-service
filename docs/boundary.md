# Development License Service Boundary

The service exists to make the remote License Provider path repeatable without
requiring a commercial licensing vendor. Its outputs are deterministic test
fixtures, not production licensing decisions.

The service is kept independent from Server business code. Server calls the
provider-neutral protocol through `license-adapter`; the service does not
import Server managers or persistence repositories.

The default state is process-local and isolated. A later issue may add a
durable production License Service, but that work must not silently change this
development service into a production authority.

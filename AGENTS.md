# droneOS Agents Guide

This repository contains Go code for a base station and drone runtime with pluggable drivers (sensors, motors, radio) and control algorithms.

## Quick orientation
- Base station entrypoint: `cmd/base/main.go`
- Drone entrypoint: `cmd/drone/main.go`
- Config: `configs/config.yaml` (base + drone settings)
- Protocol: `internal/protocol/*` (shared framing + WiFi/Radio transports)
- Radio driver: `internal/drivers/radio/SX1262`
- Sensors: `internal/drivers/sensor/*`
- Motors: `internal/drivers/motor/*`
- Control algorithms: `internal/drone/control/*`

## How to run (local dev)
- Base: `go run cmd/base/main.go -config-file configs/config.yaml`
- Drone: `go run cmd/drone/main.go -config-file configs/config.yaml`
- Both the base and drone need to be run in order to validate integration tests.

## Protocol expectations
- Messages are JSON payloads framed with a 4-byte big-endian length prefix.
- Use the shared helpers in `internal/protocol/codec.go` and `internal/protocol/transport.go`.
- Prefer WiFi when available, fall back to radio using `protocol.AutoTransport`.

## Driver patterns
- Driver `Main` functions are invoked via `utils.CallFunctionByName` with a `context.Context` as the first arg.
- Radio drivers should implement `protocol.RadioLink` (`Send([]byte)`, `Receive() ([]byte, error)`), then call `protocol.ServeRadio` to handle requests.

## Config tips
- `base.radio` and `drone.radio` map to `config.Radio` (`name`, `alwaysUse`, `usbId`, `pins`).
- `controlAlgorithmPriority` controls which control loops are started and their order.

## Logging
- Use zerolog (`github.com/rs/zerolog/log`).
- `Info` and `Error` for human output; `Debug` for machine-readable logs.

## Contribution guidelines
- Keep changes small and focused.
- Prefer adding new drivers/controls via new package directories and registering them in the maps in `cmd/drone/main.go`.
- Run `gofmt` on Go files touched.

# washmonitor-agent

A service that uses computer vision ML models to let me know when my non-smart washer is finished running

## To Do

- Add dryer cycle monitoring using a vibration sensor
- Write docs so the public can use my code

## Shelly Worker

`worker-shelly` can replace either appliance monitor using a Shelly Plus/Gen2
device. It is a sensor backend, not a separate user-facing power monitor.

Set `APPLIANCE_ROLE=washer` or `APPLIANCE_ROLE=dryer` and configure the Shelly
URL. The worker polls the device's local
`/rpc/PM1.GetStatus?id=0` endpoint over HTTP and reads the top-level
`current` value in amps. The response may also contain `apower` and cumulative
energy fields; those are intentionally ignored. Readings at or above `CURRENT_ACTIVE_THRESHOLD_A`
indicate that the machine is running; readings at or below
`CURRENT_INACTIVE_THRESHOLD_A` indicate that it is inactive. Values between
those thresholds preserve the previous state.

The worker starts counting from the first valid inactive reading, including when
monitoring begins after the appliance has already finished. It waits for
`CURRENT_INACTIVE_DURATION_SECONDS` of valid inactive readings before sending
the existing finished notification and returning the selected appliance to
`idle`. Shelly request failures mark the selected sensor unhealthy and do not
count toward the inactive duration.

To use it in Compose, disable the existing worker for the selected role and
start the optional profile with `docker compose --profile shelly up`. Only one
worker should own a washer or dryer role at a time. See
`worker-shelly/.env.example` for the available settings.

## Design

### Agent State

The agent is always in one of these 2 operational states:

- `monitor` - The agent is watching the washing machine, waiting for it to finish so that it may notify the user. It also handles API requests during this time
- `idle` - The agent is handling API calls, but not watching the washing machine

On start, the agent is in the `idle` state

### Operation Flow

Via a REST API, a user will trigger the agent to enter the `monitor` state. The agent will start watching the washer to determine when it is done.
The washing machine must be running before the agent enters the `monitor` state.

Then, once per minute, the washmonitor agent needs to determine which of these two states the subject washing machine is in:

- `running`
- `stopped`

To determine this, the agent will:

- Query a URL where it can get a JPG of the washer
- Uses a ML model to find the washer's control panel in the image
  - If it doesn't detect the presence of the control panel (usually due to poor lighting or an obstruction), it infers that the washer is currently `stopped`
- Crops the image down to just the control panel
- Uses another ML model to classify the image as `stopped` or `running`
- If the washing machine is `stopped` 5 times in a row, the agent will call a webhook intended to send a notification to the user that the washer is done. Afterwards, the agent exits the `monitor` state and returns to `idle`

### REST API

The API allows the user to interact with the agent. The user may:

- Query the operational state of the agent
- Trigger the agent's operational state to switch to `monitor`
- Trigger the agent's operational state to switch to `idle`

## Getting Started

### Prerequisites

Creating a virtual environment of Python 3.12 is recommended.

You will need to install a few dependencies, which are found in the `requirements.txt` file. Before you install those, you'll need to install pytorch. You can do this by following the instructions on the [pytorch website](https://pytorch.org/get-started/locally/). Make sure to install the version that is appropriate for your system hardware and OS.

Then, you can install the other dependencies by running:

```bash
pip install -r requirements.txt
```

### Environment Variables

- `NOTIFICATION_WEBHOOK_URL`: The URL the agent should call to send a notification to the user through a service of your choice, like Discord

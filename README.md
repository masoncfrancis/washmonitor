# washmonitor-agent
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fmasoncfrancis%2Fwashmonitor-agent.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2Fmasoncfrancis%2Fwashmonitor-agent?ref=badge_shield)


A service that uses computer vision ML models to let me know when my non-smart washer is finished running

## To Do
- Add dryer cycle monitoring using a vibration sensor
- Write docs so the public can use my code

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



## License
[![FOSSA Status](https://app.fossa.com/api/projects/git%2Bgithub.com%2Fmasoncfrancis%2Fwashmonitor-agent.svg?type=large)](https://app.fossa.com/projects/git%2Bgithub.com%2Fmasoncfrancis%2Fwashmonitor-agent?ref=badge_large)
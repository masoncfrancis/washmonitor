import requests
from dotenv import load_dotenv
from enum import Enum  # Import Enum for status validation
import proc.img as imgProc  # Import the image processing module
import proc.ml as mlProc  # Import the machine learning module
import os
import time


# Define the AgentStatus Enum
class AgentStatus(Enum):
    MONITOR = "monitor"
    IDLE = "idle"


# Define the WasherStatus Enum
class WasherStatus(Enum):
    RUNNING = "running"
    STOPPED = "stopped"


# Global vars
washerStoppedCount = 0  # Counter for stopped washing machine
agentStatus = AgentStatus.IDLE.value  # Use Enum value

def setAgentStatus(status: AgentStatus, user: str = ""):
    payload = {"status": status.value}
    if status == AgentStatus.MONITOR:
        if not user:
            raise ValueError("User is required when status is 'monitor'")
        payload["user"] = user
    requests.post(apiURL + "/washer/setAgentStatus", json=payload)
    return status.value


def getAgentStatus():
    return requests.get(apiURL + "/washer/getAgentStatus").json()["status"]

def getAgentUser():
    return requests.get(apiURL + "/washer/getAgentStatus").json()["user"]


def getWashingMachineStatus():
    if agentStatus == AgentStatus.MONITOR.value:
        print("Checking washing machine status...")

        try:
            washerImageFilePath = imgProc.getImage(os.environ.get('WASHER_CAMERA_URL'))
        except Exception as e:
            print(f"Error getting the washer image: {e}")
            return WasherStatus.STOPPED.value  # O cualquier valor seguro

        result = mlProc.cropToControlPanel(washerImageFilePath)
        if result["status"] == True:
            print("Control panel detected")
            imgProc.deleteImage(washerImageFilePath)
            classification = mlProc.classifyControlPanel(result["imagePath"])
            print("Classification result:", classification)
            imgProc.deleteImage(result["imagePath"])
            if classification == WasherStatus.STOPPED.value:
                return WasherStatus.STOPPED.value
            elif classification == WasherStatus.RUNNING.value:
                return WasherStatus.RUNNING.value
        else:
            imgProc.deleteImage(washerImageFilePath)
            print("Control panel not detected")
            return WasherStatus.STOPPED.value

    return WasherStatus.STOPPED.value  # Default to stopped


def sendDiscordNotification(message):
    requests.post(
        os.environ.get('DISCORD_URL'),
        json={"content": message}
    )


def sendSmsMessage(message, destination):
    sms_url = os.environ.get('SEND_SMS_URL')
    sms_user = os.environ.get('SMS_USER')
    sms_password = os.environ.get('SMS_PASSWORD')
    headers = {"Content-Type": "application/json"}
    data = {
        "message": message,
        "phoneNumbers": [destination]
    }
    response = requests.post(
        sms_url,
        auth=(sms_user, sms_password),
        headers=headers,
        json=data
    )

    # Print if the message was sent successfully
    if response.status_code == 200 or response.status_code == 202:
        print("SMS sent successfully")
    else:
        print(f"Failed to send SMS: {response.status_code} - {response.text}")

        

if __name__ == "__main__":

    load_dotenv()

    apiURL = os.environ.get('API_URL')

    last_washer_check = time.monotonic()
    last_agent_check = time.monotonic()

    while True:
        now = time.monotonic()

        # Check agent status every 5 seconds, always
        if now - last_agent_check >= 5:
            try:
                agentStatus = getAgentStatus()
            except Exception as e:
                print(f"Error polling agent status: {e}")
                agentStatus = agentStatus
            # Send a heartbeat/check-in to the API so server records last-seen
            try:
                requests.post(apiURL + "/washer/checkin", timeout=2)
            except Exception:
                # don't let heartbeat failures stop the loop
                pass
            last_agent_check = now

        # Check washer status every 60 seconds, only if agent is monitoring
        if agentStatus == AgentStatus.MONITOR.value and now - last_washer_check >= 60:
            washerStatus = getWashingMachineStatus()

            if washerStatus == WasherStatus.STOPPED.value:
                washerStoppedCount += 1
            elif washerStatus == WasherStatus.RUNNING.value:
                washerStoppedCount = 0

            if washerStoppedCount >= 5:
                print("Washing machine is stopped for 5 checks. Setting agent status to idle.")

                # Get the user who started the monitoring
                user = str(getAgentUser()).lower()
                print(f"User who started monitoring: {user}")

                # Set the agent status to idle
                agentStatus = setAgentStatus(AgentStatus.IDLE)
                washerStoppedCount = 0

                # Notify the user
                if user == "user2":
                    # sendDiscordNotification("✅ Washing machine has finished running")
                    destinationNumber = os.environ.get('USER2_PHONE_NUMBER')
                    sendSmsMessage("✅ Washing machine has finished running", destinationNumber)
                elif user == "user1":
                    destinationNumber = os.environ.get('USER1_PHONE_NUMBER')
                    sendSmsMessage("✅ Washing machine has finished running", destinationNumber)
            else:
                print(f"Washing machine is {washerStatus}. Agent status remains as monitor.")

            last_washer_check = now

        # Dormir hasta el siguiente evento programado
        next_agent = last_agent_check + 5
        next_washer = last_washer_check + 60 if agentStatus == AgentStatus.MONITOR.value else float('inf')
        sleep_time = max(0, min(next_agent, next_washer) - time.monotonic())
        time.sleep(sleep_time)

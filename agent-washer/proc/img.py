# Washmonitor agent image processing logic
#
# This module contains the image processing logic for the Washmonitor agent.
# It includes functions for obtaining images and cropping them

import requests
import time
import os
import uuid


def getImage(url):
    """
    Fetch an image from the given URL and save it to a temporary file.

    Args:
        url (str): The URL of the image to fetch.

    Returns:
        str: The path to the saved image file.
    """
    # Create a 'temp' directory in the current working directory if it doesn't exist
    tempDir = os.path.join(os.getcwd(), "temp")
    os.makedirs(tempDir, exist_ok=True)

    # Generate a random file name
    tempFileName = f"{uuid.uuid4()}.jpg"
    tempFilePath = os.path.join(tempDir, tempFileName)

    # Fetch the image from the URL
    response = requests.get(url)

    # Check if the request was successful
    if response.status_code == 200:
        # Save the image to the temporary file
        with open(tempFilePath, "wb") as f:
            f.write(response.content)
        return tempFilePath
    else:
        raise Exception(
            f"Failed to fetch image from {url}. Status code: {response.status_code}"
        )


def deleteImage(imagePath):
    """
    Delete the specified image file.

    Args:
        imagePath (str): The path to the image file to delete.
    """
    try:
        os.remove(imagePath)
    except Exception as e:
        print(f"Error deleting image: {e}")

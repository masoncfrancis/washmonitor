# Washmonitor agent machine learning processing logic
#
# This module contains the machine learning processing logic for the Washmonitor agent.
# It includes functions for classifying images and detecting objects in them.

from PIL import Image
from ultralytics import YOLO
import json
import os
import uuid


def cropToControlPanel(imagePath):
    """
    Determines if the image contains a control panel and crops it if found. Returns the path to the cropped image and a status indicating whether the control panel was found, in a json response.
    If it is not found, it returns the path to the original image and false status in a json response.

    Args:
        imagePath (str): The path to the image file.

    Returns:
        str: The path to the cropped image file.
    """

    status = False
    croppedImagePath = imagePath

    # Load the YOLO model for control panel detection
    print("Loading YOLO model for control panel detection...")
    model = YOLO("models/detect.pt")  # Load the YOLO model

    print("Performing inference on the image...")
    result = model(imagePath)[0]  # Perform inference on the image

    # Only proceed if there is at least one detection
    if result and hasattr(result, "boxes") and len(result.boxes) == 1:
        status = True  # Set status to true if a control panel is detected
        boxCoords = json.loads(result.to_json())[0]["box"]
        x1 = boxCoords["x1"]
        y1 = boxCoords["y1"]
        x2 = boxCoords["x2"]
        y2 = boxCoords["y2"]

        # Use pillow to crop the image
        image = Image.open(result.path)
        xSize = x2 - x1
        ySize = y2 - y1

        # Add space around bounding box to make the output image 275x215
        xBuffer = (275 - xSize) // 2
        yBuffer = 215 - ySize

        croppedImage = image.crop((x1 - xBuffer, y1 - yBuffer, x2 + xBuffer, y2))

        # Generate a random file name for the cropped image
        filename = f"{uuid.uuid4()}.jpg"

        # save file to temp directory
        tempDir = os.path.join(os.getcwd(), "temp")
        os.makedirs(tempDir, exist_ok=True)
        croppedImagePath = os.path.join(tempDir, filename)
        croppedImage.save(croppedImagePath)

    return {"imagePath": croppedImagePath, "status": status}


def classifyControlPanel(imagePath):
    """
    Classifies the control panel image and returns the classification result.

    Args:
        imagePath (str): The path to the image file.

    Returns:
        str: The classification result.
    """

    model = YOLO("models/classify.pt")  # Load the classification model
    result = model(imagePath)[0]  # Perform inference on the image

    # Get the class name from the result
    summary = result.summary()
    className = summary[0]["name"]

    return className

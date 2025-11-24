# AKA-Only-Server Web GUI User Guide

## Overview
The AKA-Only-Server Web GUI is a lightweight web interface for managing subscribers on the AKA-Only-Server. It allows you to create, view, update, and delete subscriber information (IMSI, Ki, OPC, SQN, AMF).

## Installation & Setup

1.  **Prerequisites**:
    *   AKA-Only-Server must be running.
    *   The `aka-webgui` executable.
    *   The `webgui.env` configuration file.

2.  **Configuration (`webgui.env`)**:
    *   `WEBGUI_LISTEN_ADDR`: Address to listen on (default: `localhost:9999`).
    *   `WEBGUI_AUTH_USERNAME`: Admin username (default: `admin`).
    *   `WEBGUI_AUTH_PASSWORD`: Admin password (default: `admin`).
    *   `AKA_API_BASE_URL`: URL of the AKA-Only-Server API (default: `http://localhost:8080/api/v1`).

3.  **Running the App**:
    *   **Foreground**: Simply run the executable: `./aka-webgui`
    *   **Background (Systemd)**:
        Create a service file `/etc/systemd/system/aka-webgui.service`:
        ```ini
        [Unit]
        Description=AKA-Only-Server Web GUI
        After=network.target

        [Service]
        ExecStart=/path/to/aka-webgui
        WorkingDirectory=/path/to/
        Restart=always
        User=root

        [Install]
        WantedBy=multi-user.target
        ```
        Then: `systemctl enable --now aka-webgui`

## Usage

### Login
Access the GUI at `http://localhost:9999` (or your configured address). Log in with the credentials defined in `webgui.env`.

### Dashboard
The dashboard shows the total count of subscribers and a list of existing subscribers.
*   **Note**: For security and readability, `Ki`, `OPC`, and `AMF` are hidden in the list view.

### Adding a Subscriber
1.  Click **Add Subscriber**.
2.  Fill in the details:
    *   **IMSI**: 15 digits.
    *   **Ki**: 32 hex characters.
    *   **OPC**: 32 hex characters.
    *   **SQN**: 12 hex characters.
    *   **AMF**: 4 hex characters.
3.  Click **Save**.

### Editing a Subscriber
1.  Click **Edit** next to a subscriber.
2.  Modify the fields (IMSI cannot be changed).
3.  Click **Save**.

### Deleting a Subscriber
1.  Click **Delete** next to a subscriber.
2.  Confirm the action in the browser prompt.

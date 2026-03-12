# Control Room: PoC

Welcome to the Control Room Proof of Concept (PoC). This repository contains the foundational architecture for receiving, processing, and visualizing event-driven data (Heartbeats and Users) from our various microservice teams (CRM, Facturatie, Kassa, etc.).

This guide will walk you through spinning up the local infrastructure, running the Go microservices, and building your first real-time monitoring dashboards in Kibana.

---

## 🛠️ 1. Installation & Setup

Before starting, ensure you have **Docker Desktop** open and running on your machine. The infrastructure will not spin up without it. Also ensure you're on the **feature/init** branch. Otherwise, you won't see anything in the repo.

### Step 1: Start the Infrastructure

Open your terminal in the root directory of this project (./PoC) and run:

```bash
docker-compose up -d
```

Wait about 60 seconds (or more) for the containers to fully boot. You can verify they are running by visiting:

- **RabbitMQ UI:** [http://localhost:15672/](http://localhost:15672/) _(Login: guest / guest)_
- **Elasticsearch:** [http://localhost:9200/](http://localhost:9200/)
- **Kibana:** [http://localhost:5601/](http://localhost:5601/)

### Step 2: Start the Go Microservices

Open **two separate terminal windows**.

In Terminal 1 (Start the Consumer):

```bash
cd consumer
go run consumer.go
```

In Terminal 2 (Start the Producer: Mocks external services):

```bash
cd producer
go run producer.go
```

_You should now see the producer sending messages and the consumer instantly validating and saving them to Elasticsearch._

---

## 📊 2. Kibana Setup: Creating Data Views

Kibana needs to know where to look for our data. We must create Data Views before we can build any charts.

1. Open **Kibana** at [http://localhost:5601/](http://localhost:5601/).
2. Click the **Hamburger Menu** (top left corner) and scroll down to **Stack Management**.
3. Under the Kibana section, click **Data Views**.
4. Click **Create data view**.

- **Name & Index pattern:** type `heartbeats`.
- **Timestamp field:** select `@timestamp`.
- Click **Save data view to Kibana**.

5. Repeat step 4 to create a second data view named `users`, also using `@timestamp` as the timestamp field.

---

## 🚥 3. Building the Real-Time Heartbeat Dashboard

Now we will build the live traffic light system to monitor service health.

### Step 1: Initialize the Visualization

1. Open the Hamburger Menu and go to **Dashboard**.
2. Click **Create dashboard**, then click **Create visualization**.
3. In the top-left dropdown, ensure the **`heartbeats`** data view is selected.
4. From the left-hand panel, click and drag the **`# Records`** field directly into the middle of the screen.
5. In the top chart type dropdown, change it to **Metric**.

### Step 2: Configure the Traffic Light Colors

1. On the right-hand panel under **Primary metric**, click on **Count of records** (you should be in the formula section, not quick function).
2. Scroll down to the **Color** section and change the **Color mode** to **Dyanmic**.
3. Click to edit the color rules/ranges:

- Keep the first range as **Red** and leave the condition to `>= No min`.
- Keep the second range as **Green** and set the condition to `>= 1`.

4. Go back one time, find the **Icon decoration** setting and change it to a **Heart** icon.
5. Change the **Name** (field title) to something clear, like `CRM Status`.
6. Click the back arrow (`<- Count of records`) to close the settings panel.

### Step 3: Filter for CRM & Set Timers

1. Look right to the left of the main search bar at the top and click the **`+` (Add filter)** button.

- Field: `service_id.keyword`
- Operator: `is`
- Value: `Service-CRM`
- Click **Save**.

2. In the top-right corner, change the time range to **Last 6 seconds**.
3. Left to the time range, click the **Calendar** settings, set it to **Every 6 seconds**, and click **Apply**.
4. Click **Save and return** (top right) to go back to the dashboard.

### Step 4: Do it all over again on your own for the Facturatie service

### Step 5: Save

1. On your new dashboard, click the **Save** button in the top right.
2. **CRITICAL:** Check the box that says **Store time with dashboard** before saving. Name it "Heartbeats Monitoring".
3. When looking at the two visualisations next to each other, you can see there's no title. You can easily add a title by clicking on it.

---

## 📈 4. Building the Users Analytics Dashboard

Finally, let's visualize the user creation data over time.

1. Go back to the main **Dashboards** page and click **Create dashboard**.
2. Click **Create visualization**.
3. In the top-left dropdown, change the data view from `heartbeats` to **`users`**.
4. Drag and drop **`# Records`** into the middle of the screen.
5. Ensure the chart type (top) is set to **Vertical bar stacked**.
6. Ensure **`@timestamp`** is automatically assigned to the **Horizontal axis (X-axis)** on the right panel.
7. Click on the field names in the right panel to clean up the display text (e.g., change "Count of records" to "Total Users Created").
8. Click **Save and return**.
9. Set your desired time range (e.g., _Last 30 Days_), click **Save** for the dashboard, check **Store time with dashboard**, and name it "Created Users Analytics".

---

_End of Guide. You now have a fully operational, event-driven Control Room dashboard environment running locally!_

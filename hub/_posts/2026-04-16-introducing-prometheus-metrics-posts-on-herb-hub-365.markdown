---
layout: post
title: "Introducing Prometheus Metrics Posts on Herb Hub 365"
date: 2026-04-16 07:15:09 +0000
categories: Platform Update
---

![Timelapse image for April 16, 2026](/assets/images/blog/2026-04-16-introducing-prometheus-metrics-posts-on-herb-hub-365.jpg)

We're excited to announce the introduction of Prometheus Metrics posts on Herb Hub 365! This new feature allows us to bring you a unique blend of data-driven insights and interactive visualizations, all powered by the popular Prometheus monitoring system.

**Why Prometheus?**

As our greenhouse grows and evolves, we want to ensure that we have a deep understanding of its performance and health. Prometheus is an open-source monitoring system that provides a flexible and scalable way to collect metrics from our environment. By leveraging Prometheus, we can gain valuable insights into various aspects of our greenhouse's operation, such as temperature, humidity, pressure, and more.

**How it works**

The prom-post mode is designed to take care of all the heavy lifting for us. It queries Prometheus for the desired data using a configurable JSON query file, exports each metric as a static JSON snapshot, generates a Jekyll post with interactive ECharts visualizations, and automatically commits and publishes through our existing Git pipeline.

This means that you can explore our eleven tracked metrics in real-time on the new Metrics index page, without having to worry about the underlying infrastructure. The charts are client-side rendered from committed JSON, ensuring that your experience is fast and fully static.

**The eleven metrics we track**

We're currently tracking a range of metrics to provide a comprehensive view of our greenhouse's performance. These include:

* Environment temperature
* Humidity
* Pressure
* Ambient light
* Soil moisture percentage
* Soil sensor voltage
* Probe temperatures
* Water distance
* Water reservoir percentage
* Water reservoir volume
* Last run age

**Exploring the Metrics index page**

Head over to our new Metrics index page to explore these metrics in real-time. You can zoom in and out, hover over data points, and even download the underlying data for further analysis.

We're thrilled to bring this feature to you and hope it enhances your experience on Herb Hub 365. Stay tuned for more updates and insights from our greenhouse!

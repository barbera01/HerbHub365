---
layout: post
title: "Time Lapse"
date: 2025-01-06 20:36:44 +0000
categories: Herb Hub Update
---

A quick test of creating the timelapse from the automagically created still images

```BASH
n=1
for file in [0-9]*-??-??-????.jpg; do
    printf -v new_name "%04d.jpg" "$n"
    mv -- "$file" "$new_name"
    ((n++))
done
```

```

```

<video width="640" height="360" controls>
  <source src="/assets/video/timelapse.mp4" type="video/mp4">
  Your browser does not support the video tag.
</video>

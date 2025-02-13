---
title: split
type: processor
status: stable
categories: ["Utility"]
---

<!--
     THIS FILE IS AUTOGENERATED!

     To make changes please edit the contents of:
     lib/processor/split.go
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';


Breaks message batches (synonymous with multiple part messages) into smaller batches. The size of the resulting batches are determined either by a discrete size or, if the field `byte_size` is non-zero, then by total size in bytes (which ever limit is reached first).

```yml
# Config fields, showing default values
label: ""
split:
  size: 1
  byte_size: 0
```

This processor is for breaking batches down into smaller ones. In order to break a single message out into multiple messages use the [`unarchive` processor](/docs/components/processors/unarchive).

If there is a remainder of messages after splitting a batch the remainder is also sent as a single batch. For example, if your target size was 10, and the processor received a batch of 95 message parts, the result would be 9 batches of 10 messages followed by a batch of 5 messages.

The functionality of this processor depends on being applied across messages
that are batched. You can find out more about batching [in this doc](/docs/configuration/batching).

## Fields

### `size`

The target number of messages.


Type: `int`  
Default: `1`  

### `byte_size`

An optional target of total message bytes.


Type: `int`  
Default: `0`  



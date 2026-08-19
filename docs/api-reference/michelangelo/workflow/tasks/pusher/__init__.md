---
sidebar_label: pusher
title: michelangelo.workflow.tasks.pusher
---

Michelangelo pusher — push ML artifacts to storage and registry destinations.

Public API
----------

**Dispatch:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import push

**Configuration:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import (
        PusherConfig,
        PusherPluginConfig,
        ModelPluginConfig,
        DatasetPluginConfig,
        EvalReportPluginConfig,
    )

**Results:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import PusherResult

**Plugins:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import (
        ModelPusherPlugin,
        DatasetPusherPlugin,
        EvalReportPusherPlugin,
    )

**Registry (for plugin extension):**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import default_registry, PluginRegistry

**Exceptions:**

.. code-block:: python

    from michelangelo.workflow.tasks.pusher import (
        PusherError,
        ArtifactNotFoundError,
        PusherPluginError,
        ConfigurationError,
    )

## DatasetPluginConfig

## EvalReportPluginConfig

## ModelPluginConfig

## PusherConfig

## PusherPluginConfig

## ArtifactNotFoundError

## ConfigurationError

## PusherError

## PusherPluginError

## DatasetPusherPlugin

## EvalReportPusherPlugin

## ModelPusherPlugin

## ModelPushResult

## PartialRegistrationError

## RegistrationResult

## PluginRegistry

## default\_registry

## push

## PusherResult


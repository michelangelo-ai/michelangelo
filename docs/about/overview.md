---
sidebar_position: 1
---

# Overview

Michelangelo ("MA") is Uber's machine learning platform. 

Michelangelo offers 2 primary ways for you to build and deploy your model: 

1. User Interface (UI) 
2. Opinionated predefined ML workflow (Canvas). 

## Machine Learning Tools

### Michelangelo User Interface (MA UI)
The UI environment provides a standard, code-free ML development experience. It guides users through the different phases of the ML development lifecycle. Uber’s internal term for the UI is MA Studio (you may see it appear on this site in screenshots of the UI). This environment provides all the essential tools which allows ML developers to build, train, deploy, monitor, and debug your machine learning models in a single unified visual interface to boost your productivity. 

**Currently, Preparing and Training models are available for open source users. More features will be made available soon.**

Users can use the no-code dev environment to perform standardized ML tasks without writing a single line of code, including:
* Prepare data sources for training models or making batch predictions
* Build and train XGB models, classic ML models, and Deep Learning models

### Canvas: opinionated predefined ML workflow

For more advanced tasks, such as training DL models, setting up customized retraining workflows, building bespoken model performance monitoring workflows, users can build the corresponding pipelines via Canvas, an opinionated predefined ML workflow, while managing these pipelines (running, debugging, viewing run results, etc) in the UI environment. 

Canvas provides a highly customized, code driven ML development experience by applying software development principles to ML development. Users can create their own dependencies that can be managed in the UI environment.

## Architecture

### Ecosystem Overview

At Uber, Machine Learning (ML) is critical because of the complexity and scale of all the decisions that we have to make. ML can reduce uncertainty in our processes, making our products more targeted and our user experience better. There are also so many factors and signals influencing our systems that we often need ML to make sense of what’s happening.

We make millions of predictions every second, and hundreds of applied scientists, engineers, product managers, and researchers work on ML solutions daily. Uber relies on a unique ecosystem (pictured below) of internal tooling to scale ML solutions with speed, accuracy, and reliability.

![Michelangelo Ecosystem Diagram](./images/michelangelo-ecosystem.png)

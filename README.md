# app-resource-scaler

This project uses the generic kubernetes resources [scale to zero infrastructure](https://github.com/v3io/scaler).
It implements scaling of Iguazio's app services, by modifying the appropriate CRDs. 

## Building the project

There are 2 separate docker images for `dlx` and `scaler`, each in their corresponding directories. 
To build, just run the commands (from the root dir of this project):

`docker build -f dlx/Dockerfile -t [repo]/dlx:[version] .`
`docker build -f autoscaler/Dockerfile -t [repo]/scaler:[version] .`

or run

`SCALER_TAG=[version] SCALER_REPOSITORY=iguazio/ make build`

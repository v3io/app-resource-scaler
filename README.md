# app-resource-scaler

This project serves as a plugin for the generic kubernetes resources 
[scale to zero infrastructure](https://github.com/v3io/scaler). </br>
It implements scaling of Iguazio's app services, by modifying the appropriate CRDs. 

## Building the project

There are 2 separate docker images for `dlx` and `scaler`, each in their corresponding directories. 
To build, just run the commands (from the root dir of this project): </br>

`docker build -f dlx/Dockerfile -t [repo]/dlx:[version] .` </br>
`docker build -f autoscaler/Dockerfile -t [repo]/scaler:[version] .`

or run

`SCALER_TAG=[version] SCALER_REPOSITORY=iguazio/ make build`

This will build an image of the [scaler](https://github.com/v3io/scaler) components with `resourcescaler.go` as their 
plugin. </br>
To publish the image just `docker push` the resulting images to your favorite repo.

## Notes

Modifying `vendor` dir may result in incompatability with the `vendor` dir in [scaler](https://github.com/v3io/scaler).
Please run `docker build` as described above to verify nothing was broken.

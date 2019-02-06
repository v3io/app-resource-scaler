# app-resource-scaler

This project serves as a plugin for the generic kubernetes resources scaler (https://github.com/v3io/scaler). </br>
Implements scaling of deployments, by modifying their number of replicas to be 0 (scaler) or 1 (dlx). 

## Building the project

There are 2 separate docker images for `dlx` and `scaler`, each in their corresponding directories. 
To build, just run the commands (from the root dir of this project): </br>

`docker build -f dlx/Dockerfile -t [repo]/dlx:[version] .` </br>
`docker build -f autoscaler/Dockerfile -t [repo]/scaler:[version] .`

This will build an image <a href="https://github.com/v3io/scaler"> scaler </a> with `resourcescaler.go` as its plugin. </br>
To publish the image just `docker push` the resulting images to your favorite repo.

## Notes

Modifying `vendor` dir may result in incompatability with `vendor` in <a href="https://github.com/v3io/scaler"> scaler </a>.
Please run `docker build` as described above to verify nothing was broken.

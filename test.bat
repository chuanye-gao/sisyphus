cd D:\GithubRepositories\LongWay\projects\sisyphus\bin
go build -o sisyphus.exe ..\cmd\sisyphus
copy ..\config.yaml config.yaml /y
.\sisyphus.exe

## About
cite main server

### install bra(Brilliant Ridiculous Assistant)
```
go get -u github.com/Unknwon/bra
```

### vendoring
```
go get -u github.com/tools/godep
godep save
```

### run
```
bra run
```

### test
```
http -v --timeout 600 POST localhost:8080/v1/deploy namespace="niko-bellic" service="helloworld" branch="develop" reference="6b6bfeba1397a1822ddd121200e3aa2fc037062c"

http -v --timeout 600 POST localhost:8080/v1/deploy namespace="niko-bellic" service="helloworld" branch="develop" sha="bf38aa44a1685ffbaae0afe71782b77b3b1cbd65" force_deploy="true"

http -v --timeout 600 POST localhost:8080/v1/deploy namespace="niko-bellic" service="helloworld" branch="develop" sha="bf38aa44a1685ffbaae0afe71782b77b3b1cbd65" force_deploy="false"

http -v localhost:8080/v1/deploy/test
```

### in case of slow build...
```
go get -v
```

## reference
* echo : https://echo.labstack.com
* github enterprise API : https://developer.github.com/enterprise/2.2/v3/
* httpie : https://github.com/jkbrzt/httpie
* ace : https://github.com/yosssi/ace
  * ace syntax : https://github.com/yosssi/ace/blob/master/documentation/syntax.md
* font awesome : http://fortawesome.github.io/Font-Awesome/icons/
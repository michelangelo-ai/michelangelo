## Mock interfaces

1. Add following line into the .go file that the interface(s) are defined:

```
//go:generate mamockgen interface_name1 [interface_name2 ...]
```

2. Run 'go generate .' and gazelle in the directory to generate the mocks code and BUILD.bazel
3. The package of the generated mocks code will be the original package name + mocks. For example, the mocks code generated from [api/interfaces.go](https://github.com/michelangelo-ai/michelangelo/blob/main/go/api/interface.go) is in package "github.com/michelangelo-ai/michelangelo/go/api/apimocks"   
4. commit the generated mocks code to git repo
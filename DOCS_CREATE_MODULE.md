# Create Module Use Scaffold

This document provides a step-by-step guide on how to create a new module using the Scaffold tool.

for creating a new module, follow these steps:

```go
go run.\cmd\scaffold\main.go <module_name>
```

Replace `<module_name>` with the desired name for your new module.
For example, to create a module named `example`, you would run:

```go
go run.\cmd\scaffold\main.go example
``` 

This command will generate the necessary files and directory structure for your new module in the current working
directory.
Make sure you have Go installed and properly set up on your machine to run the command successfully.
After running the command, you should see a new directory named `example` (or whatever name you provided) containing the
scaffolded files for your module.
You can then navigate into the newly created module directory and start adding your code and functionality.
For example:

```bash
cd example
```

You can now open the module in your favorite code editor and begin development.
Happy coding!
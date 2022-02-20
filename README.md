## IAM init

Iam-init is based on go-init https://github.com/pablo-ruth/go-init

When running a pod in AWS on Kubernetes, it can take a moment to set up the pod credentials if using either [IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html), [Kube2iam](https://github.com/jtblin/kube2iam), or [KIAM](https://github.com/uswitch/kiam). Iam-init checks for AWS credentials before starting up the process declared in the command.

```bash
iam-init -c "./process -opt1 -opt2 -optx"
```



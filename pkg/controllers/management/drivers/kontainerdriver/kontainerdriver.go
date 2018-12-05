package kontainerdriver

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/rancher/kontainer-engine/service"
	"github.com/rancher/kontainer-engine/types"
	"github.com/rancher/rancher/pkg/controllers/management/clusterprovisioner"
	"github.com/rancher/rancher/pkg/controllers/management/drivers"
	"github.com/rancher/rancher/pkg/controllers/management/drivers/nodedriver"
	"github.com/rancher/types/apis/core/v1"
	corev1 "github.com/rancher/types/apis/core/v1"
	"github.com/rancher/types/apis/management.cattle.io/v3"
	"github.com/rancher/types/config"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	v13 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	driverNameLabel = "io.cattle.kontainer_driver.name"
	DriverDir       = "./management-state/kontainer-drivers/"
)

var kontainerDriverName = regexp.MustCompile("kontainer-engine-driver-(.+)$")

func Register(ctx context.Context, management *config.ManagementContext) {
	lifecycle := &Lifecycle{
		dynamicSchemas:        management.Management.DynamicSchemas(""),
		dynamicSchemasLister:  management.Management.DynamicSchemas("").Controller().Lister(),
		namespaces:            management.Core.Namespaces(""),
		coreV1:                management.Core,
		kontainerDriverLister: management.Management.KontainerDrivers("").Controller().Lister(),
		kontainerDrivers:      management.Management.KontainerDrivers(""),
	}

	management.Management.KontainerDrivers("").AddLifecycle(ctx, "mgmt-kontainer-driver-lifecycle", lifecycle)
}

type Lifecycle struct {
	dynamicSchemas        v3.DynamicSchemaInterface
	dynamicSchemasLister  v3.DynamicSchemaLister
	namespaces            v1.NamespaceInterface
	coreV1                corev1.Interface
	kontainerDriverLister v3.KontainerDriverLister
	kontainerDrivers      v3.KontainerDriverInterface
}

func (l *Lifecycle) Create(obj *v3.KontainerDriver) (runtime.Object, error) {
	logrus.Infof("create kontainerdriver %v", obj.Name)

	v3.KontainerDriverConditionDownloaded.Unknown(obj)
	v3.KontainerDriverConditionInstalled.Unknown(obj)

	if !obj.Spec.Active {
		return obj, nil
	}

	// Update status
	obj, err := l.kontainerDrivers.Update(obj)
	if err != nil {
		return nil, err
	}

	if !obj.Spec.BuiltIn {
		obj, err = l.download(obj)
	} else {
		v3.KontainerDriverConditionDownloaded.True(obj)
		v3.KontainerDriverConditionInstalled.True(obj)
	}

	// Update status
	obj, err = l.kontainerDrivers.Update(obj)
	if err != nil {
		return nil, err
	}

	if hasStaticSchema(obj) {
		return obj, nil
	}

	err = l.createDynamicSchema(obj)
	if err != nil {
		return nil, err
	}

	v3.KontainerDriverConditionActive.True(obj)

	return obj, nil
}

func (l *Lifecycle) driverExists(obj *v3.KontainerDriver) bool {
	return drivers.NewKontainerDriver(obj.Spec.BuiltIn, obj.Name, obj.Spec.URL, obj.Spec.Checksum).Exists()
}

func (l *Lifecycle) download(obj *v3.KontainerDriver) (*v3.KontainerDriver, error) {
	driver := drivers.NewKontainerDriver(obj.Spec.BuiltIn, obj.Name, obj.Spec.URL, obj.Spec.Checksum)
	err := driver.Stage()
	if err != nil {
		return nil, err
	}

	v3.KontainerDriverConditionDownloaded.True(obj)

	path, err := driver.Install()
	if err != nil {
		return nil, err
	}

	v3.KontainerDriverConditionInstalled.True(obj)

	obj.Status.ExecutablePath = path
	matches := kontainerDriverName.FindStringSubmatch(path)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not parse name of kontainer driver from path: %v", path)
	}

	obj.Status.DisplayName = matches[1]
	obj.Status.ActualURL = obj.Spec.URL

	logrus.Infof("kontainerdriver %v downloaded and registered at %v", obj.Name, path)

	return obj, nil
}

func (l *Lifecycle) createDynamicSchema(obj *v3.KontainerDriver) error {
	driver := service.NewEngineService(
		clusterprovisioner.NewPersistentStore(l.namespaces, l.coreV1),
	)
	flags, err := driver.GetDriverCreateOptions(context.Background(), obj.Name, obj, v3.ClusterSpec{
		GenericEngineConfig: &v3.MapStringInterface{
			clusterprovisioner.DriverNameField: obj.Status.DisplayName,
		},
	})
	if err != nil {
		return fmt.Errorf("error getting driver create options: %v", err)
	}

	resourceFields := map[string]v3.Field{}
	for key, flag := range flags.Options {
		formattedName, field, err := toResourceField(key, flag)
		if err != nil {
			return fmt.Errorf("error formatting field name: %v", err)
		}

		resourceFields[formattedName] = field
	}

	// all drivers need a driverName field so kontainer-engine knows what type they are
	resourceFields[clusterprovisioner.DriverNameField] = v3.Field{
		Create: true,
		Update: true,
		Type:   "string",
		Default: v3.Values{
			StringValue: obj.Name,
		},
	}

	dynamicSchema := &v3.DynamicSchema{
		Spec: v3.DynamicSchemaSpec{
			SchemaName:     getDynamicTypeName(obj),
			ResourceFields: resourceFields,
		},
	}
	dynamicSchema.Name = strings.ToLower(getDynamicTypeName(obj))
	dynamicSchema.OwnerReferences = []v13.OwnerReference{
		{
			UID:        obj.UID,
			Kind:       obj.Kind,
			APIVersion: obj.APIVersion,
			Name:       obj.Name,
		},
	}
	dynamicSchema.Labels = map[string]string{}
	dynamicSchema.Labels[obj.Name] = obj.Status.DisplayName
	dynamicSchema.Labels = map[string]string{}
	dynamicSchema.Labels[driverNameLabel] = obj.Status.DisplayName
	_, err = l.dynamicSchemas.Create(dynamicSchema)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("error creating dynamic schema: %v", err)
	}

	return l.createOrUpdateKontainerDriverTypes(obj)
}

func (l *Lifecycle) createOrUpdateKontainerDriverTypes(obj *v3.KontainerDriver) error {
	nodedriver.SchemaLock.Lock()
	defer nodedriver.SchemaLock.Unlock()

	embeddedType := getDynamicTypeName(obj)
	fieldName := getDynamicFieldName(obj)

	clusterSchema, err := l.dynamicSchemasLister.Get("", "cluster")
	if err != nil && !errors.IsNotFound(err) {
		return err
	} else if errors.IsNotFound(err) {
		resourceField := map[string]v3.Field{}
		resourceField[fieldName] = v3.Field{
			Create:   true,
			Nullable: true,
			Update:   true,
			Type:     embeddedType,
		}

		dynamicSchema := &v3.DynamicSchema{}
		dynamicSchema.Name = "cluster"
		dynamicSchema.Spec.ResourceFields = resourceField
		dynamicSchema.Spec.Embed = true
		dynamicSchema.Spec.EmbedType = "cluster"
		_, err := l.dynamicSchemas.Create(dynamicSchema)
		if err != nil {
			return err
		}
		return nil
	}

	shouldUpdate := false

	if clusterSchema.Spec.ResourceFields == nil {
		clusterSchema.Spec.ResourceFields = map[string]v3.Field{}
	}
	if _, ok := clusterSchema.Spec.ResourceFields[fieldName]; !ok {
		// if embedded we add the type to schema
		clusterSchema.Spec.ResourceFields[fieldName] = v3.Field{
			Create:   true,
			Nullable: true,
			Update:   true,
			Type:     embeddedType,
		}
		shouldUpdate = true
	}

	if shouldUpdate {
		_, err = l.dynamicSchemas.Update(clusterSchema)
		if err != nil {
			return err
		}
	}

	return nil
}

func toResourceField(name string, flag *types.Flag) (string, v3.Field, error) {
	field := v3.Field{
		Create: true,
		Update: true,
		Type:   "string",
	}

	name, err := toLowerCamelCase(name)
	if err != nil {
		return name, field, err
	}

	field.Description = flag.Usage

	if flag.Type == types.StringType {
		field.Default.StringValue = flag.Value

		if flag.Password {
			field.Type = "password"
		} else {
			field.Type = "string"
		}

		if flag.Default != nil {
			field.Default.StringValue = flag.Default.DefaultString
		}
	} else if flag.Type == types.IntType {
		field.Type = "int"

		if flag.Default != nil {
			field.Default.StringValue = strconv.Itoa(int(flag.Default.DefaultInt))
		}
	} else if flag.Type == types.BoolType || flag.Type == types.BoolPointerType {
		field.Type = "boolean"

		if flag.Default != nil {
			field.Default.BoolValue = flag.Default.DefaultBool
		}
	} else if flag.Type == types.StringSliceType {
		field.Type = "array[string]"

		if flag.Default != nil {
			field.Default.StringSliceValue = flag.Default.DefaultStringSlice.Value
		}
	} else {
		return name, field, fmt.Errorf("unknown type of flag %v: %v", flag, reflect.TypeOf(flag))
	}

	return name, field, nil
}

func toLowerCamelCase(nodeFlagName string) (string, error) {
	flagNameParts := strings.Split(nodeFlagName, "-")
	flagName := flagNameParts[0]
	for _, flagNamePart := range flagNameParts[1:] {
		flagName = flagName + strings.ToUpper(flagNamePart[:1]) + flagNamePart[1:]
	}
	return flagName, nil
}

func (l *Lifecycle) Updated(obj *v3.KontainerDriver) (runtime.Object, error) {
	logrus.Infof("update kontainerdriver %v", obj.Name)
	if obj.Spec.BuiltIn {
		return obj, nil
	}

	if hasStaticSchema(obj) {
		return obj, nil
	}

	// redownload file if url changed or not downloaded
	var err error
	if obj.Spec.URL != obj.Status.ActualURL || v3.KontainerDriverConditionDownloaded.IsFalse(obj) || !l.driverExists(obj) {
		obj, err = l.download(obj)
		if err != nil {
			return nil, err
		}
	}

	if obj.Spec.Active {
		err = l.createDynamicSchema(obj)

		v3.KontainerDriverConditionActive.True(obj)
	} else {
		if err = l.dynamicSchemas.Delete(getDynamicTypeName(obj), &v13.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return nil, fmt.Errorf("error deleting schema: %v", err)
		}

		if err = l.removeFieldFromCluster(obj); err != nil {
			return nil, err
		}
	}

	return obj, err
}

func hasStaticSchema(obj *v3.KontainerDriver) bool {
	return obj.Name == "rancherKubernetesEngine" || obj.Name == "import"
}

func getDynamicTypeName(obj *v3.KontainerDriver) string {
	var s string
	if obj.Spec.BuiltIn {
		s = obj.Status.DisplayName + "Config"
	} else {
		s = obj.Status.DisplayName + "EngineConfig"
	}

	return s
}

func getDynamicFieldName(obj *v3.KontainerDriver) string {
	if obj.Spec.BuiltIn {
		return obj.Status.DisplayName + "Config"
	}

	return obj.Status.DisplayName + "EngineConfig"
}

func (l *Lifecycle) Remove(obj *v3.KontainerDriver) (runtime.Object, error) {
	logrus.Infof("remove kontainerdriver %v", obj.Name)

	driver := drivers.NewKontainerDriver(obj.Spec.BuiltIn, obj.Name, obj.Spec.URL, obj.Spec.Checksum)
	err := driver.Remove()
	if err != nil {
		return nil, err
	}

	if hasStaticSchema(obj) {
		return obj, nil
	}

	if err := l.dynamicSchemas.Delete(getDynamicTypeName(obj), &v13.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error deleting dynamic schema: %v", err)
	}

	if err := l.removeFieldFromCluster(obj); err != nil {
		return nil, err
	}

	return obj, nil
}

func (l *Lifecycle) removeFieldFromCluster(obj *v3.KontainerDriver) error {
	nodedriver.SchemaLock.Lock()
	defer nodedriver.SchemaLock.Unlock()

	fieldName := getDynamicFieldName(obj)

	nodeSchema, err := l.dynamicSchemasLister.Get("", "cluster")
	if err != nil {
		return fmt.Errorf("error getting schema: %v", err)
	}

	delete(nodeSchema.Spec.ResourceFields, fieldName)

	if _, err = l.dynamicSchemas.Update(nodeSchema); err != nil {
		return fmt.Errorf("error removing schema from cluster: %v", err)
	}

	return nil
}

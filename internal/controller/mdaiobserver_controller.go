package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff/v5"
	mdaiv1 "github.com/decisiveai/mdai-operator/api/v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ Controller = (*MdaiObserverReconciler)(nil)

// MdaiObserverReconciler reconciles a MdaiObserver object
type MdaiObserverReconciler struct {
	client.Client

	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	greptime Greptime
}

// +kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiobservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiobservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hub.mydecisive.ai,resources=mdaiobservers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *MdaiObserverReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logger.FromContext(ctx)
	log.Info("-- Starting MdaiObserver reconciliation --", "namespace", req.NamespacedName, "name", req.Name)

	fetchedCR := &mdaiv1.MdaiObserver{}
	if err := r.Get(ctx, req.NamespacedName, fetchedCR); err != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		if !apierrors.IsNotFound(err) {
			log.Error(err, "unable to fetch MdaiObserver CR:"+req.Namespace+" : "+req.Name)
		}
		log.Info("-- Exiting MdaiObserver reconciliation, CR is deleted already --")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	_, err := r.ReconcileHandler(ctx, *NewObserverAdapter(fetchedCR, log, r.Client, r.Recorder, r.Scheme, r.greptime))
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("-- Finished MdaiObserver reconciliation --")

	return ctrl.Result{}, nil
}

// ReconcileHandler processes the MdaiObserver CR and performs the necessary operations.
func (*MdaiObserverReconciler) ReconcileHandler(ctx context.Context, adapter Adapter) (ctrl.Result, error) {
	observerAdapter, ok := adapter.(ObserverAdapter)
	if !ok {
		return ctrl.Result{}, fmt.Errorf("unexpected adapter type: %T", adapter)
	}

	operations := []ReconcileOperation{
		observerAdapter.ensureDeletionProcessed,
		observerAdapter.ensureStatusInitialized,
		observerAdapter.ensureFinalizerInitialized,
		observerAdapter.ensureSynchronized,
		observerAdapter.ensureStatusSetToDone,
	}
	for _, operation := range operations {
		result, err := operation(ctx)
		if err != nil || result.RequeueRequest {
			return ctrl.Result{RequeueAfter: result.RequeueDelay}, err
		}
		if result.CancelRequest {
			return ctrl.Result{}, nil
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MdaiObserverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.initializeGreptimeDb(); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&mdaiv1.MdaiObserver{}).
		Named("mdaiobserver").
		Complete(r)
}

func (r *MdaiObserverReconciler) initializeGreptimeDb() error {
	ctx := context.Background()
	log := logger.FromContext(ctx)
	retryCount := 0

	greptimeHost := os.Getenv("GREPTIME_HOST")
	greptimePort := ""
	if greptimePort = os.Getenv("GREPTIME_PORT"); greptimePort != "" {
		greptimePort = "4003"
	}
	greptimePassword := os.Getenv("GREPTIME_PASSWORD")
	if greptimeHost == "" || greptimePassword == "" {
		return errors.New("GREPTIME_HOST and GREPTIME_PASSWORD environment variables must be set to enable GreptimeDB client")
	}
	log.Info("Initializing GreptimeDB  client", "host", greptimeHost, "port", greptimePort)
	operation := func() (string, error) {
		// TODO: add password
		dsn := fmt.Sprintf("host=%s port=%s dbname=public sslmode=disable", greptimeHost, greptimePort)
		greptimeDb, err := gorm.Open(postgres.New(postgres.Config{
			DSN:              dsn,
			WithoutReturning: true,
		}), &gorm.Config{
			DisableAutomaticPing: true,
		})
		if err != nil {
			retryCount++
			log.Error(err, "Failed to initialize Greptime  client. Retrying...")
			return "", err
		}
		r.greptime = Greptime{greptimeDb: *greptimeDb, greptimeTemplates: loadTemplates()}
		return "", nil
	}

	exponentialBackoff := backoff.NewExponentialBackOff()
	exponentialBackoff.InitialInterval = 5 * time.Second //nolint:mnd

	notifyFunc := func(err error, duration time.Duration) {
		log.Error(err, "Failed to initialize Greptime client. Retrying...", "retry_count", retryCount, "duration", duration.String())
	}

	if _, err := backoff.Retry(context.TODO(), operation,
		backoff.WithBackOff(exponentialBackoff),
		backoff.WithMaxElapsedTime(3*time.Minute), //nolint:mnd
		backoff.WithNotify(notifyFunc),
	); err != nil {
		return fmt.Errorf("failed to initialize Greptime client after retries: %w", err)
	}
	return nil
}

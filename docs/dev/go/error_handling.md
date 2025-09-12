# Golang Error Handling Code Review Checklist

## Core Principles

### ✅ Errors as Values
- Errors are treated as values, not control flow mechanisms
- No panics for expected failures or recoverable conditions
- Error handling follows Go's idiomatic `if err != nil` pattern

### ✅ Error Propagation Strategy
*✅ Production Reality: Different layers have different logging responsibilities*

**Three-Layer Approach:**
- **Domain/Business Logic**: Return enriched errors without logging (focus on correctness)
- **Service/Controller Layer**: Log AND return for operational visibility (production requirement)
- **Transport Layer**: Always log with full request context (debugging essential)

**Error Context Requirements:**
- Errors are wrapped with context using `fmt.Errorf("operation: %w", err)`
- Error messages include operation context and relevant identifiers
- Include correlation IDs (request IDs, resource names, namespaces)
- **No secrets or PII in error messages**

```go
// ✅ Domain/Business Logic - return enriched errors only
func (s *Service) Put(ctx context.Context, key string, value interface{}) error {
    if err := s.validateInput(key, value); err != nil {
        return fmt.Errorf("validate input for key %q: %w", key, err)
    }
    
    if err := s.storage.Store(key, value); err != nil {
        return fmt.Errorf("store key %q: %w", key, err)
    }
    
    return nil
}

// ✅ Controller/Service Layer - LOG AND RETURN for operational visibility
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx).WithValues("resource", req.NamespacedName)
    
    if err := r.service.Put(ctx, req.Name, data); err != nil {
        logger.Error(err, "failed to store resource", 
            "operation", "reconcile_store",
            "namespace", req.Namespace,
            "name", req.Name)
        return ctrl.Result{RequeueAfter: time.Minute}, err  // Log AND return
    }
    
    logger.Info("resource stored successfully")
    return ctrl.Result{}, nil
}

// ✅ Transport Layer - always log with full context
func (h *HTTPHandler) UpdateResource(w http.ResponseWriter, r *http.Request) {
    requestID := middleware.GetRequestID(r.Context())
    logger := log.WithFields(log.Fields{"request_id": requestID})
    
    err := h.controller.Reconcile(r.Context(), req)
    if err != nil {
        logger.Error("request failed",
            "error", err,
            "method", r.Method,
            "path", r.URL.Path,
            "request_id", requestID)
        http.Error(w, "internal server error", 500)
        return
    }
}
```

## Error Classification

### ✅ Retryable vs Non-Retryable Error Classification

Classify errors to determine if operations should be retried automatically:

**Retryable Errors** (safe to retry automatically):
- Temporary network failures (connection timeouts, DNS resolution failures)
- Service unavailable errors (HTTP 503, temporary database connection issues)
- Rate limiting errors (HTTP 429)
- Transient infrastructure failures

**Non-Retryable Errors** (should not be retried):
- Input validation failures (malformed JSON, invalid parameters)
- Authentication/authorization failures (HTTP 401, 403)
- Resource not found errors (HTTP 404)
- Business logic violations (insufficient funds, duplicate records)

**Implementation Guidelines:**
- Error types should implement interfaces for classification
- Use specific error types rather than string matching
- Document retry behavior clearly in API specifications

**References:**
- [Go Blog: Error Handling and Go](https://blog.golang.org/error-handling-and-go)
- [Kubernetes API Conventions - Errors](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#success-codes)
- [gRPC Error Handling Guide](https://grpc.io/docs/guides/error/)

```go
type RetryableError struct {
    Cause error
    After time.Duration
}

func (e *RetryableError) Error() string {
    return fmt.Sprintf("retryable error: %v", e.Cause)
}

func (e *RetryableError) Unwrap() error {
    return e.Cause
}
```

## Error Context and Messages

### ✅ Meaningful Error Messages
- Include operation being performed
- Include relevant identifiers (IDs, names, keys)
- Provide enough context for debugging
- Follow consistent format: `"operation identifier: cause"`

```go
// ✅ Good
return fmt.Errorf("failed to update user %q in database: %w", userID, err)
return fmt.Errorf("invalid configuration for service %q: missing required field 'endpoint'", serviceName)

// ❌ Bad
return fmt.Errorf("error: %w", err)
return errors.New("something went wrong")
```

### ✅ Security Considerations
- **Never include secrets, passwords, or tokens in error messages**
- **Never include PII (personally identifiable information)**
- Sanitize user input before including in error messages
- Use generic identifiers where possible

```go
// ✅ Good
return fmt.Errorf("authentication failed for user %q", userID)

// ❌ Bad
return fmt.Errorf("authentication failed: invalid password %q", password)
return fmt.Errorf("failed to process email %q", email)  // PII
```

## Error Handling Patterns

### ✅ Strategic Error Logging
*✅ Production Reality: Operational visibility often requires logging at multiple levels*

**Preferred Pattern** - Three-Layer Strategy:
- **Domain Layer**: Return enriched errors, no logging (pure business logic)
- **Service Layer**: Log significant business events + return errors (operational boundaries)  
- **Transport Layer**: Always log with full request context (HTTP handlers, controllers)

**When to Log AND Return** (Production Best Practice):
- **Controllers/Reconcilers**: Always log errors for operational debugging
- **Service boundaries**: Where business logic meets infrastructure
- **External integrations**: API calls, database operations, message queues
- **Resource operations**: Kubernetes API calls, file I/O, network operations
- **Critical paths**: Where error loss would impact system reliability

**Implementation Guidelines:**
- Include correlation IDs (request IDs, user IDs, resource identifiers)
- Use structured logging with relevant context
- Classify errors: validation (don't log) vs system errors (always log)
- Choose appropriate log levels (ERROR for actionable issues)

```go
// ✅ Good - log at boundary (Kubernetes/controller-runtime pattern)
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
    user, err := h.service.CreateUser(ctx, req)
    if err != nil {
        logger.Error("failed to create user",
            "correlation_id", correlationID,
            "user_id", req.UserID,
            "error", err)
        http.Error(w, "internal server error", 500)
        return
    }
}

// Service layer - don't log, just return (matches Kubernetes business logic)
func (s *Service) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    if err := s.repo.Save(ctx, user); err != nil {
        return nil, fmt.Errorf("save user %q: %w", user.ID, err)  // No logging here
    }
}

// ✅ Real Kubernetes boundary logging example:
func (rsc *ReplicaSetController) processNextWorkItem(ctx context.Context) bool {
    err := rsc.syncHandler(ctx, key)  // calls business logic
    if err != nil {
        utilruntime.HandleError(fmt.Errorf("sync %q failed with %v", key, err))  // LOG HERE
        rsc.queue.AddRateLimited(key)
    }
    return true
}
```

### ✅ Error Wrapping Strategy
*When to wrap vs when to return directly*

**Error Wrapping Guidelines:**
- **Controllers/Services**: Always wrap errors with operation and resource context
- **Business Logic**: Wrap when crossing architectural boundaries
- **Utility Functions**: Usually return directly (context obvious or will be wrapped higher)
- **External Calls**: Always wrap with operation context

**Error Wrapping Benefits:**
- **Debugging Speed**: "create spark job ml-team/training: connection refused" vs "connection refused"
- **Resource Identification**: Know exactly which resource failed in busy clusters
- **Operation Context**: Understand the call chain that led to failure

**When to Use Each Pattern:**

**✅ Wrap Errors** (for operational context):
```go
// Controllers - always wrap with resource context
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) error {
    if err := r.createResource(ctx, resource); err != nil {
        return fmt.Errorf("reconcile %s %s: %w", resource.Kind, req.NamespacedName, err)
    }
}

// Services - wrap with business context  
func (s *Service) ProcessOrder(orderID string) error {
    if err := s.validateOrder(orderID); err != nil {
        return fmt.Errorf("process order %s: %w", orderID, err)
    }
}

// External integrations - wrap with operation context
func (c *Client) CreateSparkJob(job *SparkJob) error {
    if err := c.httpClient.Post(url, job); err != nil {
        return fmt.Errorf("create spark job %s/%s: %w", job.Namespace, job.Name, err)
    }
}
```

**✅ Direct Return** (when context is obvious):
```go
// Validation functions
func validateEmail(email string) error {
    if !emailRegex.MatchString(email) {
        return ErrInvalidEmail  // Context is obvious
    }
}

// Simple getters (Kubernetes errors already have context)
func (r *Reconciler) getResource(ctx context.Context, name string) (*Resource, error) {
    resource := &Resource{}
    err := r.Get(ctx, types.NamespacedName{Name: name}, resource)
    return resource, err  // K8s error already includes resource info
}

// Internal utilities (will be wrapped by caller)
func connectDatabase(url string) (*sql.DB, error) {
    return sql.Open("postgres", url)  // Business layer will wrap
}
```

**Implementation Requirements:**
- Use `fmt.Errorf` with `%w` verb for error wrapping
- Preserve original error for `errors.Is()` and `errors.As()` checks
- Include resource identifiers (namespace/name) in controller operations
- Add operation context ("create", "update", "delete", "get")
- Don't wrap nil errors

```go
// ✅ Good
func (s *Service) ProcessOrder(ctx context.Context, orderID string) error {
    order, err := s.repo.GetOrder(ctx, orderID)
    if err != nil {
        return fmt.Errorf("get order %q: %w", orderID, err)
    }
    
    if err := s.payment.Charge(ctx, order); err != nil {
        return fmt.Errorf("charge payment for order %q: %w", orderID, err)
    }
    
    return nil
}
```

## Anti-Patterns to Avoid
*❌ These patterns are NOT found in Kubernetes, Controller-Runtime, or major Go projects*

### ❌ Common Mistakes in Error Handling
- **Logging without context** - Generic error messages without correlation IDs
- **Inconsistent logging strategy** - Some errors logged, others not, no clear pattern
- **Double-wrapping errors** - Wrapping already wrapped errors unnecessarily  
- **Swallowing errors** - Ignoring errors with `_ = err`
- **Generic error messages** - "something went wrong", "error occurred"
- **Exposing internal details** - Returning database errors directly to API clients
- **Missing operational context** - Not logging enough detail for debugging production issues

```go
// ❌ Bad - logging without context
if err != nil {
    log.Error("error occurred", err)  // No correlation ID, operation, or resource info
    return fmt.Errorf("operation failed: %w", err)
}

// ❌ Bad - swallowing errors
result, _ := someOperation()  // Error information lost

// ❌ Bad - no context in error messages
return err  // No operation context, resource identifiers

// ❌ Bad - inconsistent logging (some errors logged, some not)
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) error {
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        return err  // Not logged - might be lost
    }
    
    if err := r.Update(ctx, obj); err != nil {
        logger.Error("update failed", err)  // This one is logged
        return err
    }
}

// ✅ Good - consistent operational logging
func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) error {
    logger := log.FromContext(ctx).WithValues("resource", req.NamespacedName)
    
    if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
        logger.Error(err, "failed to get resource", "operation", "get")
        return fmt.Errorf("get resource %q: %w", req.NamespacedName, err)
    }
    
    if err := r.Update(ctx, obj); err != nil {
        logger.Error(err, "failed to update resource", "operation", "update")
        return fmt.Errorf("update resource %q: %w", req.NamespacedName, err)
    }
    
    return nil
}
```

## Testing Error Handling

### ✅ Error Testing
- Test both success and error paths
- Verify error messages contain expected context
- Test error wrapping and unwrapping with `errors.Is()` and `errors.As()`
- Mock dependencies to simulate different error conditions

```go
func TestService_ProcessOrder_PaymentFailure(t *testing.T) {
    // Setup mocks
    paymentErr := errors.New("payment declined")
    mockPayment.EXPECT().Charge(gomock.Any(), gomock.Any()).Return(paymentErr)
    
    // Test
    err := service.ProcessOrder(ctx, "order-123")
    
    // Verify
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "charge payment for order \"order-123\"")
    assert.True(t, errors.Is(err, paymentErr))
}
```

## Performance Considerations

### ✅ Error Performance
- Avoid creating errors in hot paths when not needed
- Use pre-allocated sentinel errors for common cases
- Consider error pooling for high-frequency operations
- Don't format expensive strings unless error will be returned

```go
// ✅ Good - sentinel errors
var (
    ErrUserNotFound = errors.New("user not found")
    ErrInvalidInput = errors.New("invalid input")
)

// ✅ Good - conditional formatting
func validateInput(input string) error {
    if len(input) == 0 {
        return ErrInvalidInput  // Pre-allocated
    }
    if len(input) > maxLength {
        return fmt.Errorf("input too long: %d > %d", len(input), maxLength)
    }
    return nil
}
```


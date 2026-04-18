# Model Resolution Implementation

## Overview
Miniagent now automatically resolves model string aliases to their canonical model IDs on startup using the auto provider.

## Changes Made

### 1. Added `resolveModel()` function in `main.go`
**Location**: Lines 292-315 in main.go

The function:
- Takes a provider and a model string (alias or ID)
- Searches through all available models from the provider
- Matches on exact ID or any model alias
- Returns the canonical model ID
- Returns an error with a helpful message if model is not found

```go
func resolveModel(provider llm.Provider, modelStr string) (string, error) {
	if modelStr == "" {
		return "", fmt.Errorf("model string cannot be empty")
	}

	// Search through available models
	for _, m := range provider.Models() {
		// Exact match on ID
		if m.ID == modelStr {
			return m.ID, nil
		}
		// Match on aliases
		for _, alias := range m.Aliases {
			if alias == modelStr {
				return m.ID, nil
			}
		}
	}

	// Not found
	return "", fmt.Errorf("unknown model: %q (use 'miniagent models' to list available models)", modelStr)
}
```

### 2. Added Model Resolution in `execute()` function
**Location**: Lines 256-263 in main.go

After creating the provider and before building the agent, the code now:
- Checks if a model string was provided
- Resolves it to a canonical ID using `resolveModel()`
- Returns an error if resolution fails
- Updates `inference.Model` with the canonical ID

```go
// Resolve model string to canonical ID
if inference.Model != "" {
	resolvedModel, err := resolveModel(provider, inference.Model)
	if err != nil {
		return err
	}
	inference.Model = resolvedModel
}
```

## Usage Examples

### Using an alias
```bash
miniagent -m opus "list all files in current directory"
```
- `"opus"` is resolved to the full canonical ID (e.g., `claude-opus-4-5-20250514`)

### Using "default" alias
```bash
miniagent -m default "hello"
```
- Resolves to the default model alias

### Using full canonical ID (unchanged)
```bash
miniagent -m claude-opus-4-5-20250514 "hello"
```
- Already resolved, no additional lookup needed

### Getting available models
```bash
miniagent models
```
- Lists all available model IDs and their aliases

## Testing

A comprehensive test suite was added in `model_resolution_test.go` with the following test cases:

1. ✓ Exact ID match
2. ✓ Alias match - "opus"
3. ✓ Alias match - "default"
4. ✓ Alias match - "sonnet"
5. ✓ Alias match - "haiku"
6. ✓ Unknown model error handling
7. ✓ Empty string error handling

All tests pass successfully:
```
=== PASS: TestResolveModel (0.00s)
    --- PASS: TestResolveModel/exact_ID_match (0.00s)
    --- PASS: TestResolveModel/alias_match_-_opus (0.00s)
    --- PASS: TestResolveModel/alias_match_-_default (0.00s)
    --- PASS: TestResolveModel/alias_match_-_sonnet (0.00s)
    --- PASS: TestResolveModel/alias_match_-_haiku (0.00s)
    --- PASS: TestResolveModel/unknown_model (0.00s)
    --- PASS: TestResolveModel/empty_string (0.00s)
```

## Benefits

1. **User-friendly**: Users can use short, memorable aliases instead of full model IDs
2. **Early validation**: Model names are resolved at startup, catching errors before any work begins
3. **Cleaner logs**: Logs show canonical IDs for consistency and clarity
4. **Auto provider integration**: Leverages the existing auto provider's model discovery

## Implementation Details

- **Auto Provider**: Uses the auto provider that's already integrated to get the model catalog
- **No breaking changes**: Fully backward compatible - exact IDs still work as before
- **Error handling**: Clear error messages guide users to use `miniagent models` if a model is not found
- **Efficient**: Single pass through models at startup (minimal performance impact)

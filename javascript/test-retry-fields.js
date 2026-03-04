// Simple test to check RetryInfo schema without dependencies
console.log('=== Testing RetryInfo field schema ===');

// Let's check the decoded protobuf data directly
const protoData = "CiZtaWNoZWxhbmdlbG8vYXBpL3YyL3BpcGVsaW5lX3J1bi5wcm90bxITbWljaGVsYW5nZWxvLmFwaS52MiK5BQoPUGlwZWxpbmVSdW5TcGVjElwKCHBpcGVsaW5lGAEgASgLMiQubWljaGVsYW5nZWxvLmFwaS5SZXNvdXJjZUlkZW50aWZpZXJCJNroAiAKHm1pY2hlbGFuZ2Vsby51YmVyLmNvbS9QaXBlbGluZRJcCghyZXZpc2lvbhgFIAEoCzIkLm1pY2hlbGFuZ2Vsby5hcGkuUmVzb3VyY2VJZGVudGlmaWVyQiTa6AIgCh5taWNoZWxhbmdlbG8udWJlci5jb20vUmV2aXNpb24SVgoFZHJhZnQYBiABKAsyJC5taWNoZWxhbmdlbG8uYXBpLlJlc291cmNlSWRlbnRpZmllckIh2ugCHQobbWljaGVsYW5nZWxvLnViZXIuY29tL0RyYWZ0EiwKBWFjdG9yGAIgASgLMh0ubWljaGVsYW5nZWxvLmFwaS52Mi5Vc2VySW5mbxIMCgRraWxsGAMgASgIEiYKBWlucHV0GAQgASgLMhcuZ29vZ2xlLnByb3RvYnVmLlN0cnVjdBIrCgZyZXN1bWUYByABKAsyGy5taWNoZWxhbmdlbG8uYXBpLnYyLlJlc3VtZRISCgpsb2NhbF9kaWZmGAggASgJEjAKEndvcmtzcGFjZV9yb290X2RpchgJIAEoCUIU0ugCEEIOXigvW1x3LV0rKSkvPyQSEwoLZGVzY3JpcHRpb24YCiABKAkSOAoNbm90aWZpY2F0aW9ucxgLIAMoCzIhLm1pY2hlbGFuZ2Vsby5hcGkudjIuTm90aWZpY2F0aW9uEjgKDXBpcGVsaW5lX3NwZWMYDCABKAsyIS5taWNoZWxhbmdlbG8uYXBpLnYyLlBpcGVsaW5lU3BlYxIyCgpyZXRyeV9pbmZvGA0gASgLMh4ubWljaGVsYW5nZWxvLmFwaS52Mi5SZXRyeUluZm8iXgoJUmV0cnlJbmZvEhMKC2FjdGl2aXR5X2lkGAEgASgJEhMKC3dvcmtmbG93X2lkGAIgASgJEg4KBnJlYXNvbhgDIAEoCRIXCg93b3JrZmxvd19ydW5faWQYBCABKAk";

console.log('Protobuf data contains:');
console.log('- activity_id field definition found:', protoData.includes('YWN0aXZpdHlfaWQ')); // base64 for "activity_id"

// Also let's check the actual proto file content
import { readFileSync } from 'fs';

try {
  const protoContent = readFileSync('../proto/api/v2/pipeline_run.proto', 'utf8');
  console.log('\n=== From pipeline_run.proto ===');

  const retryInfoMatch = protoContent.match(/message RetryInfo \{([^}]+)\}/s);
  if (retryInfoMatch) {
    console.log('RetryInfo message definition:');
    console.log(retryInfoMatch[1].trim());
  }
} catch (error) {
  console.log('Could not read proto file:', error.message);
}

console.log('\n=== Expected field names ===');
console.log('Based on protobuf definition, fields should be:');
console.log('- activity_id (snake_case)');
console.log('- workflow_id (snake_case)');
console.log('- reason');
console.log('- workflow_run_id (snake_case)');

console.log('\n🔍 The issue is likely that the frontend is using camelCase field names');
console.log('   but protobuf expects snake_case field names to match the .proto definition.');
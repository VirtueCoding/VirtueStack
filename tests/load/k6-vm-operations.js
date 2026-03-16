/**
 * VirtueStack k6 Load Test Script
 * 
 * Tests VM operations under load with 200 concurrent users.
 * Includes ramp up/down phases and various operation types.
 * 
 * Usage:
 *   k6 run tests/load/k6-vm-operations.js
 * 
 * Environment variables:
 *   BASE_URL       - Target base URL (default: http://localhost:8080)
 *   CUSTOMER_TOKEN - JWT token for customer authentication
 *   ADMIN_TOKEN    - JWT token for admin authentication
 *   VUS            - Number of virtual users (default: 200)
 *   DURATION       - Test duration (default: 5m)
 */

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

// Custom metrics
const errorRate = new Rate('errors');
const vmCreateTrend = new Trend('vm_create_duration');
const vmListTrend = new Trend('vm_list_duration');
const vmStartTrend = new Trend('vm_start_duration');
const vmStopTrend = new Trend('vm_stop_duration');
const authTrend = new Trend('auth_duration');
const requestCounter = new Counter('requests_total');

// Configuration
const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const CUSTOMER_TOKEN = __ENV.CUSTOMER_TOKEN || '';
const ADMIN_TOKEN = __ENV.ADMIN_TOKEN || '';
const TEST_VM_ID = __ENV.TEST_VM_ID || '00000000-0000-0000-0000-000000000001';

// Track VMs created during the test for cleanup
const createdVMIds = [];

// Test configuration
export const options = {
    // Test stages with ramp up/down
    stages: [
        // Ramp up to 50 users over 30 seconds
        { duration: '30s', target: 50 },
        // Stay at 50 users for 1 minute
        { duration: '1m', target: 50 },
        // Ramp up to 100 users over 30 seconds
        { duration: '30s', target: 100 },
        // Stay at 100 users for 2 minutes
        { duration: '2m', target: 100 },
        // Ramp up to 200 users over 30 seconds
        { duration: '30s', target: 200 },
        // Stay at 200 users for 3 minutes (peak load)
        { duration: '3m', target: 200 },
        // Ramp down to 100 users over 30 seconds
        { duration: '30s', target: 100 },
        // Ramp down to 0 users over 30 seconds
        { duration: '30s', target: 0 },
    ],
    
    // Performance thresholds
    thresholds: {
        // Overall error rate should be less than 1%
        errors: ['rate<0.01'],
        
        // 95% of requests should complete within these times
        http_req_duration: ['p(95)<500', 'p(99)<1000'],
        
        // Specific operation thresholds
        'vm_create_duration': ['p(95)<2000', 'p(99)<3000'],
        'vm_list_duration': ['p(95)<500', 'p(99)<800'],
        'vm_start_duration': ['p(95)<1000', 'p(99)<2000'],
        'vm_stop_duration': ['p(95)<1000', 'p(99)<2000'],
        'auth_duration': ['p(95)<500', 'p(99)<800'],
        
        // Request success rates
        'checks': ['rate>0.99'],
    },
    
    // Tag all requests with test info
    tags: {
        test: 'vm-operations-load-test',
        environment: __ENV.K6_ENV || 'development',
    },
};

// Test data
const testPlans = [
    { id: 'plan-basic', name: 'Basic Plan', vcpu: 1, memory: 2048, disk: 20 },
    { id: 'plan-standard', name: 'Standard Plan', vcpu: 2, memory: 4096, disk: 50 },
    { id: 'plan-premium', name: 'Premium Plan', vcpu: 4, memory: 8192, disk: 100 },
];

const testTemplates = [
    { id: 'ubuntu-22-04', name: 'Ubuntu 22.04' },
    { id: 'ubuntu-20-04', name: 'Ubuntu 20.04' },
    { id: 'debian-12', name: 'Debian 12' },
    { id: 'centos-9', name: 'CentOS Stream 9' },
    { id: 'rocky-9', name: 'Rocky Linux 9' },
];

// Helper functions
function getAuthHeaders(token = CUSTOMER_TOKEN) {
    return {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token}`,
        'X-Correlation-ID': randomString(16),
    };
}

function generateHostname() {
    return `load-test-${randomString(8)}-${Date.now()}`;
}

function generatePassword() {
    return `P@ss${randomString(12)}!`;
}

// Test scenarios
export default function () {
    // Random sleep between iterations (realistic user think time)
    sleep(randomIntBetween(1, 5));
    
    // Randomly select operation type based on weighted distribution
    const operation = Math.random();
    
    if (operation < 0.3) {
        // 30% - List VMs (most common operation)
        listVMsTest();
    } else if (operation < 0.5) {
        // 20% - View VM details
        getVMDetailsTest();
    } else if (operation < 0.65) {
        // 15% - Start VM
        startVMTest();
    } else if (operation < 0.8) {
        // 15% - Stop VM
        stopVMTest();
    } else if (operation < 0.9) {
        // 10% - Authentication
        authTest();
    } else {
        // 10% - Create VM (less frequent due to resource usage)
        createVMTest();
    }
}

// Test: List VMs
function listVMsTest() {
    group('List VMs', () => {
        const url = `${BASE_URL}/api/v1/vms`;
        const params = {
            headers: getAuthHeaders(),
            tags: { operation: 'list_vms' },
        };
        
        const response = http.get(url, params);
        requestCounter.add(1);
        
        const success = check(response, {
            'list_vms: status is 200': (r) => r.status === 200,
            'list_vms: response has data': (r) => {
                try {
                    const body = JSON.parse(r.body);
                    return body.data !== undefined;
                } catch {
                    return false;
                }
            },
            'list_vms: response time < 500ms': (r) => r.timings.duration < 500,
        });
        
        errorRate.add(!success);
        vmListTrend.add(response.timings.duration);
    });
}

// Test: Get VM Details
function getVMDetailsTest() {
    group('Get VM Details', () => {
        const url = `${BASE_URL}/api/v1/vms/${TEST_VM_ID}`;
        const params = {
            headers: getAuthHeaders(),
            tags: { operation: 'get_vm' },
        };
        
        const response = http.get(url, params);
        requestCounter.add(1);
        
        const success = check(response, {
            'get_vm: status is 200 or 404': (r) => r.status === 200 || r.status === 404,
            'get_vm: response time < 500ms': (r) => r.timings.duration < 500,
        });
        
        errorRate.add(!success);
    });
}

// Test: Start VM
function startVMTest() {
    group('Start VM', () => {
        const url = `${BASE_URL}/api/v1/vms/${TEST_VM_ID}/start`;
        const params = {
            headers: getAuthHeaders(),
            tags: { operation: 'start_vm' },
        };
        
        const response = http.post(url, null, params);
        requestCounter.add(1);
        
        const success = check(response, {
            'start_vm: status is 200, 202, or 409': (r) => 
                r.status === 200 || r.status === 202 || r.status === 409,
            'start_vm: response time < 1000ms': (r) => r.timings.duration < 1000,
        });
        
        errorRate.add(!success);
        vmStartTrend.add(response.timings.duration);
    });
}

// Test: Stop VM
function stopVMTest() {
    group('Stop VM', () => {
        const url = `${BASE_URL}/api/v1/vms/${TEST_VM_ID}/stop`;
        const params = {
            headers: getAuthHeaders(),
            tags: { operation: 'stop_vm' },
        };
        
        const response = http.post(url, null, params);
        requestCounter.add(1);
        
        const success = check(response, {
            'stop_vm: status is 200, 202, or 409': (r) => 
                r.status === 200 || r.status === 202 || r.status === 409,
            'stop_vm: response time < 1000ms': (r) => r.timings.duration < 1000,
        });
        
        errorRate.add(!success);
        vmStopTrend.add(response.timings.duration);
    });
}

// Test: Authentication
function authTest() {
    group('Authentication', () => {
        // Test login endpoint
        const loginUrl = `${BASE_URL}/api/v1/auth/login`;
        const loginPayload = JSON.stringify({
            email: `load-test-${randomString(8)}@test.com`,
            password: generatePassword(),
        });
        
        const loginResponse = http.post(loginUrl, loginPayload, {
            headers: { 'Content-Type': 'application/json' },
            tags: { operation: 'login' },
        });
        requestCounter.add(1);
        
        // Login should fail for non-existent user, but still test the endpoint
        check(loginResponse, {
            'login: returns expected status': (r) => 
                r.status === 401 || r.status === 400 || r.status === 429,
            'login: response time < 500ms': (r) => r.timings.duration < 500,
        });
        
        authTrend.add(loginResponse.duration);
        
        // Test token refresh endpoint (with invalid token)
        const refreshUrl = `${BASE_URL}/api/v1/auth/refresh`;
        const refreshPayload = JSON.stringify({
            refresh_token: randomString(64),
        });
        
        const refreshResponse = http.post(refreshUrl, refreshPayload, {
            headers: { 'Content-Type': 'application/json' },
            tags: { operation: 'refresh_token' },
        });
        requestCounter.add(1);
        
        check(refreshResponse, {
            'refresh: returns 401 for invalid token': (r) => r.status === 401,
        });
    });
}

// Test: Create VM
function createVMTest() {
    group('Create VM', () => {
        const plan = testPlans[Math.floor(Math.random() * testPlans.length)];
        const template = testTemplates[Math.floor(Math.random() * testTemplates.length)];
        
        const url = `${BASE_URL}/api/v1/vms`;
        const payload = JSON.stringify({
            hostname: generateHostname(),
            plan_id: plan.id,
            template_id: template.id,
            password: generatePassword(),
            ssh_keys: [],
        });
        
        const params = {
            headers: getAuthHeaders(),
            tags: { operation: 'create_vm' },
        };
        
        const response = http.post(url, payload, params);
        requestCounter.add(1);
        
        const success = check(response, {
            'create_vm: status is 201, 202, or 400': (r) => 
                r.status === 201 || r.status === 202 || r.status === 400,
            'create_vm: response time < 2000ms': (r) => r.timings.duration < 2000,
        });
        
        errorRate.add(!success);
        vmCreateTrend.add(response.timings.duration);
        
        // If VM was created successfully, record ID for cleanup
        if (response.status === 201 || response.status === 202) {
            try {
                const body = JSON.parse(response.body);
                if (body.data && body.data.id) {
                    createdVMIds.push(body.data.id);
                    console.log(`Created VM: ${body.data.id}`);
                }
            } catch {
                // Ignore parse errors
            }
        }
    });
}

// Setup function (runs once per VU)
export function setup() {
    console.log('Starting VirtueStack load test...');
    console.log(`Base URL: ${BASE_URL}`);
    console.log(`Target: 200 concurrent users`);
    
    // Verify API is accessible
    const healthResponse = http.get(`${BASE_URL}/health`);
    
    if (healthResponse.status !== 200) {
        console.log('Warning: Health check failed');
    } else {
        console.log('Health check passed');
    }
    
    return { startTime: Date.now() };
}

// Teardown function (runs once after all VUs complete)
export function teardown(data) {
    const duration = (Date.now() - data.startTime) / 1000;
    console.log(`Load test completed in ${duration.toFixed(2)} seconds`);

    // Cleanup VMs created during the test
    if (createdVMIds.length > 0 && ADMIN_TOKEN) {
        console.log(`Cleaning up ${createdVMIds.length} VM(s) created during load test...`);
        for (const vmId of createdVMIds) {
            const deleteUrl = `${BASE_URL}/api/v1/admin/vms/${vmId}`;
            const deleteResponse = http.del(deleteUrl, null, {
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${ADMIN_TOKEN}`,
                },
                tags: { operation: 'cleanup_vm' },
            });
            if (deleteResponse.status === 200 || deleteResponse.status === 202 || deleteResponse.status === 204) {
                console.log(`Cleaned up VM: ${vmId}`);
            } else {
                console.warn(`Failed to clean up VM ${vmId}: status ${deleteResponse.status}`);
            }
        }
    } else if (createdVMIds.length > 0 && !ADMIN_TOKEN) {
        console.warn(`Skipping cleanup of ${createdVMIds.length} VM(s): ADMIN_TOKEN not set. Set ADMIN_TOKEN to enable automatic cleanup.`);
    }
}

// Handle summary
export function handleSummary(data) {
    return {
        'load-test-summary.json': JSON.stringify(data, null, 2),
        stdout: textSummary(data, { indent: ' ', enableColors: true }),
    };
}

// Text summary helper
function textSummary(data, options = {}) {
    const indent = options.indent || '';
    const colors = options.enableColors || false;
    
    let summary = '\n' + indent + '==========================================\n';
    summary += indent + 'VirtueStack Load Test Summary\n';
    summary += indent + '==========================================\n\n';
    
    // HTTP metrics
    if (data.metrics.http_req_duration) {
        summary += indent + 'HTTP Request Duration:\n';
        summary += indent + `  Avg: ${(data.metrics.http_req_duration.values.avg).toFixed(2)}ms\n`;
        summary += indent + `  P95: ${(data.metrics.http_req_duration.values['p(95)']).toFixed(2)}ms\n`;
        summary += indent + `  P99: ${(data.metrics.http_req_duration.values['p(99)']).toFixed(2)}ms\n`;
    }
    
    // Error rate
    if (data.metrics.errors) {
        const errorPercent = (data.metrics.errors.values.rate * 100).toFixed(2);
        summary += indent + `\nError Rate: ${errorPercent}%\n`;
    }
    
    // Request count
    if (data.metrics.requests_total) {
        summary += indent + `\nTotal Requests: ${data.metrics.requests_total.values.count}\n`;
    }
    
    // Iteration count
    if (data.metrics.iterations) {
        summary += indent + `Total Iterations: ${data.metrics.iterations.values.count}\n`;
    }
    
    summary += indent + '\n==========================================\n';
    
    return summary;
}
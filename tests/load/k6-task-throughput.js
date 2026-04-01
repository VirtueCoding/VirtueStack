/**
 * VirtueStack k6 task throughput load test.
 *
 * Exercises:
 * - POST /api/v1/provisioning/vms/:id/power (task-producing operation)
 * - GET /api/v1/provisioning/tasks/:id (task status fetch throughput)
 *
 * Usage:
 *   k6 run tests/load/k6-task-throughput.js
 *
 * Required environment variables:
 *   PROVISIONING_API_KEY
 *   TEST_VM_ID
 *
 * Optional environment variables:
 *   BASE_URL
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const PROVISIONING_API_KEY = __ENV.PROVISIONING_API_KEY || '';
const TEST_VM_ID = __ENV.TEST_VM_ID || '';
const operations = ['start', 'stop', 'restart'];

const taskCreateDuration = new Trend('task_create_duration');
const taskStatusDuration = new Trend('task_status_duration');
const taskThroughputErrors = new Rate('task_throughput_errors');
const taskCreateRequests = new Counter('task_create_requests');
const taskStatusRequests = new Counter('task_status_requests');

export const options = {
	stages: [
		{ duration: '30s', target: 20 },
		{ duration: '60s', target: 60 },
		{ duration: '90s', target: 120 },
		{ duration: '30s', target: 0 },
	],
	thresholds: {
		http_req_duration: ['p(95)<500'],
		http_req_failed: ['rate<0.01'],
		task_create_duration: ['p(95)<1000'],
		task_status_duration: ['p(95)<500'],
		task_throughput_errors: ['rate<0.01'],
	},
	tags: {
		test: 'task-throughput-load-test',
		environment: __ENV.K6_ENV || 'development',
	},
};

function headers() {
	return {
		'Content-Type': 'application/json',
		'X-API-Key': PROVISIONING_API_KEY,
	};
}

function pickOperation() {
	return operations[randomIntBetween(0, operations.length - 1)];
}

function parseTaskID(response) {
	try {
		const body = JSON.parse(response.body);
		return body && body.data && body.data.task_id ? body.data.task_id : '';
	} catch {
		return '';
	}
}

export default function () {
	const createResponse = http.post(
		`${BASE_URL}/api/v1/provisioning/vms/${TEST_VM_ID}/power`,
		JSON.stringify({ operation: pickOperation() }),
		{ headers: headers(), tags: { operation: 'task_create_power' } },
	);
	taskCreateRequests.add(1);
	taskCreateDuration.add(createResponse.timings.duration);

	const createOK = check(createResponse, {
		'task create: status is 200 or accepted conflict/validation': (r) =>
			r.status === 200 || r.status === 400 || r.status === 409,
		'task create: response time < 1200ms': (r) => r.timings.duration < 1200,
	});

	let statusOK = true;
	const taskID = parseTaskID(createResponse);
	if (taskID) {
		const statusResponse = http.get(
			`${BASE_URL}/api/v1/provisioning/tasks/${taskID}`,
			{ headers: headers(), tags: { operation: 'task_status_get' } },
		);
		taskStatusRequests.add(1);
		taskStatusDuration.add(statusResponse.timings.duration);

		statusOK = check(statusResponse, {
			'task status: status is 200': (r) => r.status === 200,
			'task status: has task payload': (r) => {
				try {
					const body = JSON.parse(r.body);
					return body && body.data && body.data.id !== undefined;
				} catch {
					return false;
				}
			},
			'task status: response time < 600ms': (r) => r.timings.duration < 600,
		});
	}

	taskThroughputErrors.add(!(createOK && statusOK));
	sleep(randomIntBetween(1, 2));
}

export function setup() {
	if (!PROVISIONING_API_KEY) {
		throw new Error('PROVISIONING_API_KEY is required');
	}
	if (!TEST_VM_ID) {
		throw new Error('TEST_VM_ID is required');
	}
}

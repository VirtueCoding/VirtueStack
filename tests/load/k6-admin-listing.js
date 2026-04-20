/**
 * VirtueStack k6 admin listing load test.
 *
 * Exercises:
 * - GET /api/v1/admin/vms with filters + pagination
 * - GET /api/v1/admin/customers with filters + pagination
 *
 * Usage:
 *   k6 run tests/load/k6-admin-listing.js
 *
 * Required environment variables:
 *   ADMIN_TOKEN
 *
 * Optional environment variables:
 *   BASE_URL
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const ADMIN_TOKEN = __ENV.ADMIN_TOKEN || '';
const vmStatuses = ['provisioning', 'running', 'stopped', 'suspended', 'migrating', 'error', 'deleted'];
const customerStatuses = ['active', 'pending_verification', 'suspended', 'deleted'];
const searchTerms = ['', 'prod', 'web', 'db', 'test'];

const adminListingDuration = new Trend('admin_listing_duration');
const adminListingErrors = new Rate('admin_listing_errors');
const adminListingRequests = new Counter('admin_listing_requests');

export const options = {
	stages: [
		{ duration: '30s', target: 20 },
		{ duration: '60s', target: 50 },
		{ duration: '60s', target: 100 },
		{ duration: '30s', target: 0 },
	],
	thresholds: {
		http_req_duration: ['p(95)<500'],
		http_req_failed: ['rate<0.01'],
		admin_listing_duration: ['p(95)<600'],
		admin_listing_errors: ['rate<0.01'],
	},
	tags: {
		test: 'admin-listing-load-test',
		environment: __ENV.K6_ENV || 'development',
	},
};

function headers() {
	return {
		'Content-Type': 'application/json',
		Authorization: `Bearer ${ADMIN_TOKEN}`,
	};
}

function vmListQuery() {
	const params = new URLSearchParams({
		page: String(randomIntBetween(1, 20)),
		per_page: String([10, 20, 50][randomIntBetween(0, 2)]),
		status: vmStatuses[randomIntBetween(0, vmStatuses.length - 1)],
	});
	const search = searchTerms[randomIntBetween(0, searchTerms.length - 1)];
	if (search) {
		params.set('search', search);
	}
	return params.toString();
}

function customerListQuery() {
	const params = new URLSearchParams({
		page: String(randomIntBetween(1, 20)),
		per_page: String([10, 20, 50][randomIntBetween(0, 2)]),
		status: customerStatuses[randomIntBetween(0, customerStatuses.length - 1)],
	});
	const search = searchTerms[randomIntBetween(0, searchTerms.length - 1)];
	if (search) {
		params.set('search', search);
	}
	return params.toString();
}

function assertListResponse(response, endpoint) {
	const success = check(response, {
		[`${endpoint}: status is 200`]: (r) => r.status === 200,
		[`${endpoint}: has data and meta`]: (r) => {
			try {
				const body = JSON.parse(r.body);
				return body && body.data !== undefined && body.meta !== undefined;
			} catch {
				return false;
			}
		},
		[`${endpoint}: response time < 800ms`]: (r) => r.timings.duration < 800,
	});
	adminListingErrors.add(!success);
	adminListingDuration.add(response.timings.duration);
}

export default function () {
	const listVMsResponse = http.get(
		`${BASE_URL}/api/v1/admin/vms?${vmListQuery()}`,
		{ headers: headers(), tags: { operation: 'admin_list_vms' } },
	);
	adminListingRequests.add(1);
	assertListResponse(listVMsResponse, 'admin list vms');

	const listCustomersResponse = http.get(
		`${BASE_URL}/api/v1/admin/customers?${customerListQuery()}`,
		{ headers: headers(), tags: { operation: 'admin_list_customers' } },
	);
	adminListingRequests.add(1);
	assertListResponse(listCustomersResponse, 'admin list customers');

	sleep(randomIntBetween(1, 2));
}

export function setup() {
	if (!ADMIN_TOKEN) {
		throw new Error('ADMIN_TOKEN is required');
	}
}

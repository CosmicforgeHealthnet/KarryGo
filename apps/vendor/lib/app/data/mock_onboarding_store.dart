import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;

enum MockOnboardingScenario {
  draft('New business'),
  pendingReview('Pending review'),
  actionRequired('Action required'),
  expired('Expired verification'),
  suspended('Suspended business'),
  ready('Ready for activation'),
  activated('Activated business'),
  permissionDenied('Setup permission denied'),
  unavailable('Verification service unavailable');

  const MockOnboardingScenario(this.label);
  final String label;
}

class MockOnboardingStore {
  MockOnboardingStore(this.vendorId, this.scenario) {
    business = {
      'id': vendorId,
      'legal_name': 'Karryam Demo Business',
      'trading_name': '',
      'support_email': 'support@example.com',
      'support_phone': '+2348012345678',
      'currency': 'NGN',
      'timezone': 'Africa/Lagos',
      'status': scenario == MockOnboardingScenario.activated
          ? 'ACTIVE'
          : scenario == MockOnboardingScenario.suspended
          ? 'SUSPENDED'
          : 'DRAFT',
      'version': 1,
    };
    if (scenario != MockOnboardingScenario.draft) {
      representative = {
        'id': 'mock-representative',
        'business_id': vendorId,
        'capacity': 'OWNER',
        'first_name': 'Ada',
        'last_name': 'Okafor',
        'email': 'ada@example.com',
        'phone': '+2348012345678',
        'authorized': true,
        'active': true,
        'version': 1,
      };
      branches.add({
        'id': 'mock-branch',
        'business_id': vendorId,
        'display_name': 'Main branch',
        'address': {
          'line1': '10 Example Road',
          'city': 'Lagos',
          'region': 'Lagos',
          'country_code': 'NG',
        },
        'latitude': 6.5,
        'longitude': 3.4,
        'timezone': 'Africa/Lagos',
        'status': 'DRAFT',
        'version': 1,
      });
      capabilities.add({
        'id': 'mock-capability',
        'business_id': vendorId,
        'capability_type': 'RESTAURANT',
        'status': 'DRAFT',
        'version': 1,
      });
      bank = {
        'business_id': vendorId,
        'bank_code': '044',
        'masked_account': '******6789',
        'resolved_account_name': 'KARRYAM DEMO BUSINESS',
        'status': 'VERIFIED',
        'version': 1,
      };
      for (final dimension in ['IDENTITY', 'BUSINESS', 'BANK', 'LOCATION']) {
        verifications.add({
          'id': 'mock-verification-$dimension',
          'business_id': vendorId,
          'dimension': dimension,
          'subject_type': dimension == 'IDENTITY'
              ? 'REPRESENTATIVE'
              : dimension == 'LOCATION'
              ? 'BRANCH'
              : 'BUSINESS',
          'subject_ref': dimension == 'IDENTITY'
              ? 'mock-representative'
              : dimension == 'LOCATION'
              ? 'mock-branch'
              : vendorId,
          'status':
              scenario == MockOnboardingScenario.ready ||
                  scenario == MockOnboardingScenario.activated
              ? 'VERIFIED'
              : scenario == MockOnboardingScenario.actionRequired
              ? 'REJECTED'
              : scenario == MockOnboardingScenario.expired
              ? 'EXPIRED'
              : scenario == MockOnboardingScenario.suspended
              ? 'SUSPENDED'
              : 'PENDING_REVIEW',
          'remediation': scenario == MockOnboardingScenario.actionRequired
              ? 'Upload a clear, current document and resubmit.'
              : '',
          'version': 1,
        });
      }
      if (scenario == MockOnboardingScenario.activated) {
        branches.first['status'] = 'ACTIVE';
        branches.add({
          ...branches.first,
          'id': 'mock-branch-east',
          'display_name': 'Lekki branch',
        });
        verifications.add({
          ...verifications.firstWhere((v) => v['dimension'] == 'LOCATION'),
          'id': 'mock-verification-LOCATION-east',
          'subject_ref': 'mock-branch-east',
        });
        capabilities.first['status'] = 'ACTIVE';
        capabilities.add({
          'id': 'mock-shop-capability',
          'business_id': vendorId,
          'capability_type': 'SHOP_GROCERY',
          'status': 'ACTIVE',
          'version': 1,
        });
        capabilities.add({
          'id': 'mock-cloud-kitchen-capability',
          'business_id': vendorId,
          'capability_type': 'CLOUD_KITCHEN',
          'status': 'ACTIVE',
          'version': 1,
        });
        capabilities.add({
          'id': 'mock-pastry-capability',
          'business_id': vendorId,
          'capability_type': 'PASTRY_BAKERY',
          'status': 'ACTIVE',
          'version': 1,
        });
        for (final branch in branches) {
          branch['capabilities'] = [
            for (final capability in capabilities)
              {
                'capability_id': capability['id'],
                'capability_type': capability['capability_type'],
                'status': 'ACTIVE',
                'version': 1,
              },
          ];
        }
      }
      status = scenario == MockOnboardingScenario.activated
          ? 'ACTIVATED'
          : scenario == MockOnboardingScenario.actionRequired
          ? 'ACTION_REQUIRED'
          : 'PENDING_REVIEW';
    }
  }
  final String vendorId;
  final MockOnboardingScenario scenario;
  late Map<String, dynamic> business;
  Map<String, dynamic> representative = {}, bank = {};
  final List<Map<String, dynamic>> branches = [],
      brands = [],
      capabilities = [],
      documents = [],
      verifications = [];
  final Map<String, Map<String, dynamic>> _replays = {}, _uploads = {};
  final Map<String, String> _grants = {};
  final Map<String, DateTime> _grantExpiry = {};
  final Set<String> _completedUploads = {};
  bool needsBusiness = false;
  String status = 'NOT_STARTED';
  String? _challenge, _purpose;
  DateTime? _challengeExpires;
  int _sequence = 0;
  static const types = [
    'RESTAURANT',
    'FAST_FOOD',
    'CLOUD_KITCHEN',
    'PASTRY_BAKERY',
    'SHOP_GROCERY',
    'MARKET',
    'WHOLESALE',
    'CATERING',
    'MANUFACTURER',
  ];
  Map<String, dynamic> get readiness {
    final requirements = <Map<String, dynamic>>[
      {
        'code': 'BUSINESS_BASICS',
        'scope': 'BUSINESS',
        'satisfied': (business['legal_name'] as String).isNotEmpty,
        'status': (business['legal_name'] as String).isNotEmpty
            ? 'VERIFIED'
            : 'NOT_STARTED',
      },
      {
        'code': 'AUTHORIZED_REPRESENTATIVE',
        'scope': 'BUSINESS',
        'satisfied': representative['authorized'] == true,
        'status': representative['authorized'] == true
            ? 'VERIFIED'
            : 'NOT_STARTED',
      },
      {
        'code': 'FIRST_BRANCH',
        'scope': 'BUSINESS',
        'satisfied': branches.isNotEmpty,
        'status': branches.isNotEmpty ? 'VERIFIED' : 'NOT_STARTED',
      },
      for (final dim in ['IDENTITY', 'BUSINESS', 'BANK']) _requirement(dim),
      for (final branch in branches) _requirement('LOCATION', branch['id']),
    ];
    final blockers = requirements.where((r) => r['satisfied'] != true).toList();
    return {
      'ready': blockers.isEmpty,
      'progress_percent':
          (100 * (requirements.length - blockers.length) / requirements.length)
              .round(),
      'requirements': requirements,
      'blockers': blockers,
    };
  }

  Map<String, dynamic> _requirement(String dim, [Object? subject]) {
    final matches = verifications.where(
      (v) =>
          v['dimension'] == dim &&
          (subject == null || v['subject_ref'] == subject),
    );
    final state = matches.isEmpty ? 'NOT_STARTED' : matches.first['status'];
    return {
      'code': 'VERIFY_$dim',
      'scope': dim == 'LOCATION' ? 'BRANCH' : 'BUSINESS',
      'dimension': dim,
      'subject_ref': dim == 'IDENTITY'
          ? representative['id']
          : dim == 'LOCATION'
          ? subject ?? (branches.isEmpty ? null : branches.first['id'])
          : vendorId,
      'satisfied': state == 'VERIFIED',
      'status': state,
      if (state != 'VERIFIED')
        'blocker':
            '$dim verification is ${state.toString().toLowerCase().replaceAll('_', ' ')}.',
    };
  }

  Map<String, dynamic> get overview => {
    'status': status,
    'version': 1,
    if (representative.isNotEmpty) 'representative': representative,
    'verifications': verifications,
    'activation_readiness': readiness,
  };
  http.StreamedResponse handle(
    http.BaseRequest req,
    Map<String, dynamic> body,
  ) {
    final path = req.url.path, method = req.method;
    if (scenario == MockOnboardingScenario.permissionDenied) {
      return error(403, 'You do not have permission to manage onboarding.');
    }
    if (scenario == MockOnboardingScenario.unavailable) {
      return error(
        503,
        'Verification service is unavailable. Try again later.',
      );
    }
    if (path.contains('/step-up/')) {
      if (path.endsWith('/start')) {
        _purpose = body['purpose'];
        _challenge = 'mock-step-${++_sequence}';
        _challengeExpires = DateTime.now().add(const Duration(minutes: 5));
        return ok({'challenge_id': _challenge, 'expires_in': 300});
      }
      if (body['purpose'] != _purpose ||
          body['challenge_id'] != _challenge ||
          body['otp'] != '123456' ||
          _challengeExpires == null ||
          DateTime.now().isAfter(_challengeExpires!)) {
        return error(401, 'Invalid or expired security code.');
      }
      final grant = 'mock-grant-${++_sequence}';
      _grants[grant] = _purpose!;
      _grantExpiry[grant] = DateTime.now().add(const Duration(minutes: 5));
      _challenge = null;
      return ok({'grant_id': grant, 'expires_in': 300});
    }
    if (path.endsWith('/capability-types')) {
      return ok([
        for (final t in types)
          {
            'type': t,
            'family': ['SHOP_GROCERY', 'MARKET', 'WHOLESALE'].contains(t)
                ? 'SHOP'
                : t == 'MANUFACTURER'
                ? 'GOODS'
                : 'FOOD',
          },
      ]);
    }
    if (path.contains('/businesses/') &&
        !path.contains('/businesses/$vendorId')) {
      return error(404, 'Business not found.');
    }
    if (method == 'GET') {
      if (path.contains('/branches/')) {
        final parts = path.split('/branches/').last.split('/');
        final found = branches.where((b) => b['id'] == parts.first);
        if (found.isEmpty) return error(404, 'Branch not found.');
        final branch = found.first;
        if (parts.length == 1) return ok(branch);
        switch (parts[1]) {
          case 'hours':
            return ok(branch['hours'] ?? []);
          case 'service-area':
            return ok(branch['service_area']);
          case 'operations-settings':
            return ok(branch['operations_settings']);
          case 'capabilities':
            return ok(branch['capabilities'] ?? []);
        }
      }

      if (path.contains('/document-uploads/')) {
        final ref = path.split('/').last;
        if (!_uploads.containsKey(ref)) return error(404, 'Upload not found.');
        return ok({
          'media_ref': ref,
          'upload_status': _completedUploads.contains(ref)
              ? 'COMPLETED'
              : 'PENDING',
          'safety_status': _completedUploads.contains(ref) ? 'SAFE' : 'PENDING',
        });
      }

      if (path.endsWith('/bank-profile')) return ok(bank.isEmpty ? null : bank);
      if (path.endsWith('/documents')) return ok(documents);

      if (path.endsWith('/representative')) {
        return ok(representative.isEmpty ? null : representative);
      }
      if (path.contains('/verifications')) {
        return ok(
          verifications
              .where(
                (v) =>
                    path.endsWith('/verifications') ||
                    path.endsWith('/${v['dimension']}'),
              )
              .toList(),
        );
      }
      if (path.endsWith('/activation-readiness') ||
          path.endsWith('/checklist') ||
          path.endsWith('/blockers')) {
        return ok(readiness);
      }
      if (path.endsWith('/onboarding') || path.endsWith('/progress')) {
        return ok(overview);
      }
      if (path.endsWith('/branches')) return ok({'items': branches});
      if (path.endsWith('/brands')) return ok({'items': brands});
      if (path.endsWith('/capabilities')) return ok({'items': capabilities});
      if (path.endsWith('/businesses/$vendorId')) {
        return needsBusiness ? error(404, 'Business not found.') : ok(business);
      }
      return error(404, 'Resource not found.');
    }
    final key =
        req.headers['Idempotency-Key'] ?? req.headers['idempotency-key'];
    if (key == null || key.isEmpty) {
      return error(400, 'An idempotency key is required.');
    }
    final fingerprint = jsonEncode([method, path, body]);
    if (_replays.containsKey(key)) {
      final replay = _replays[key]!;
      if (replay['fingerprint'] != fingerprint) {
        return error(409, 'This request key was used with different data.');
      }
      return ok(replay['data'], status: replay['status']);
    }
    http.StreamedResponse commit(Object? data, {int status = 200}) {
      _replays[key] = {
        'fingerprint': fingerprint,
        'data': jsonDecode(jsonEncode(data)),
        'status': status,
      };
      return ok(data, status: status);
    }

    if (method == 'POST' && path == '/api/v1/vendor/businesses') {
      if (!needsBusiness) {
        return error(
          409,
          'Your business details already exist. Refresh to continue.',
        );
      }
      if ((body['legal_name']?.toString().trim().length ?? 0) < 2 ||
          !RegExp(r'^[A-Z]{3}$').hasMatch(body['currency']?.toString() ?? '') ||
          (body['timezone']?.toString().isEmpty ?? true)) {
        return error(422, 'Check your business details.');
      }
      business.addAll(body);
      business['status'] = 'DRAFT';
      business['version'] = 1;
      needsBusiness = false;
      return commit(business, status: 201);
    }
    bool version(Map<String, dynamic> old) =>
        body['expected_version'] == (old['version'] ?? 0);
    if (path.contains('/branches/')) {
      final parts = path.split('/branches/').last.split('/');
      final found = branches.where((b) => b['id'] == parts.first);
      if (found.isEmpty) return error(404, 'Branch not found.');
      final branch = found.first;
      if (parts.length > 1) {
        if (parts[1] == 'hours') {
          if (!version(branch)) {
            return error(
              409,
              'Branch hours have changed. Refresh before saving.',
            );
          }
          branch['hours'] = body['hours'];
          branch['version']++;
          return commit(branch['hours']);
        }
        if (parts[1] == 'service-area' || parts[1] == 'operations-settings') {
          final field = parts[1] == 'service-area'
              ? 'service_area'
              : 'operations_settings';
          final old = branch[field] as Map<String, dynamic>? ?? {};
          if (!version(old)) {
            return error(
              409,
              'Branch settings have changed. Refresh before saving.',
            );
          }
          branch[field] = {...body, 'version': (old['version'] ?? 0) + 1};
          return commit(branch[field]);
        }
        if (parts[1] == 'capabilities' && parts.length == 3) {
          final caps =
              branch.putIfAbsent('capabilities', () => <Map<String, dynamic>>[])
                  as List;
          final existing = caps.where(
            (v) => v['vendor_capability_id'] == parts[2],
          );
          if (existing.isNotEmpty) {
            return error(409, 'This business type is already linked.');
          }
          final cap = capabilities.where((v) => v['id'] == parts[2]);
          if (cap.isEmpty) return error(404, 'Business type not found.');
          final linked = {
            'id': 'mock-branch-capability-${++_sequence}',
            'business_id': vendorId,
            'branch_id': branch['id'],
            'vendor_capability_id': parts[2],
            'capability_type': cap.first['capability_type'],
            'status': 'DRAFT',
            'effective': false,
            'effective_reason': 'BUSINESS_NOT_ACTIVE',
            'version': 1,
          };
          caps.add(linked);
          return commit(linked);
        }
      }
    }
    if (path.endsWith('/representative')) {
      if (!version(representative)) {
        return error(
          409,
          'The representative has changed. Refresh before saving.',
        );
      }
      if (verifications.any(
            (v) => v['dimension'] == 'IDENTITY' && v['status'] == 'VERIFIED',
          ) &&
          _grants.remove(body['step_up_grant_id']) !=
              'onboarding.representative_replace') {
        return error(
          403,
          'Representative replacement needs security verification.',
        );
      }
      representative = {
        ...body,
        'id': representative['id'] ?? 'mock-representative',
        'business_id': vendorId,
        'active': true,
        'version': (representative['version'] ?? 0) + 1,
      }..remove('step_up_grant_id');
      return commit(representative);
    }
    if (path.endsWith('/bank-profile')) {
      if (!version(bank)) {
        return error(409, 'Bank details have changed. Refresh before saving.');
      }
      if (_grants.remove(body['step_up_grant_id']) != 'finance.bank_change') {
        return error(403, 'Verify a security code before saving bank details.');
      }
      final account = body['account_number'] as String;
      bank = {
        'business_id': vendorId,
        'bank_code': body['bank_code'],
        'masked_account': '******${account.substring(account.length - 4)}',
        'resolved_account_name': 'KARRYAM DEMO BUSINESS',
        'status': 'PENDING_REVIEW',
        'version': (bank['version'] ?? 0) + 1,
      };
      return commit(bank);
    }
    if (path.endsWith('/document-uploads')) {
      final mime = body['mime_type'], size = body['byte_size'];
      if (![
            'image/jpeg',
            'image/png',
            'image/webp',
            'application/pdf',
          ].contains(mime) ||
          size is! int ||
          size <= 0 ||
          size > 10485760) {
        return error(400, 'Unsupported upload.');
      }
      final ref = 'mock-media-${++_sequence}';
      final grant = {
        'upload_session_id': 'mock-upload-$_sequence',
        'media_ref': ref,
        'upload_url':
            'https://mock.karryam.invalid/api/v1/media-files/vendor-uploads/$ref/content',
        'upload_token': 'mock-upload-token-$_sequence',
        'expires_at': DateTime.now()
            .add(const Duration(minutes: 5))
            .toUtc()
            .toIso8601String(),
        'max_bytes': size,
        'allowed_mime_types': [mime],
      };
      _uploads[ref] = grant;
      return commit(grant, status: 201);
    }
    if (path.endsWith('/documents')) {
      if (body['expected_version'] != 0 ||
          !_completedUploads.contains(body['media_ref'])) {
        return error(400, 'Choose and upload a document first.');
      }
      final doc = {
        ...body,
        'id': 'mock-document-${++_sequence}',
        'business_id': vendorId,
        'status': 'DRAFT',
        'version': 1,
      };
      documents.add(doc);
      return commit(doc, status: 201);
    }
    if (path.contains('/documents/')) {
      final found = documents.where((d) => path.endsWith('/${d['id']}'));
      if (found.isEmpty) return error(404, 'Document not found.');
      final doc = found.first;
      if (doc['status'] != 'DRAFT' || !version(doc)) {
        return error(409, 'Only the latest draft document can be changed.');
      }
      if (method == 'DELETE') {
        documents.remove(doc);
        return commit(null, status: 204);
      }
      for (final scope in ['representative_id', 'branch_id', 'brand_id']) {
        doc.remove(scope);
      }
      doc.addAll({...body, 'version': doc['version'] + 1});
      return commit(doc);
    }
    if (path.contains('/verifications/')) {
      final dim = path.split('/')[6];
      final refs = body['document_refs'] as List? ?? [];
      if (refs.isEmpty ||
          refs.any(
            (ref) =>
                !documents.any((d) => d['id'] == ref && d['dimension'] == dim),
          )) {
        return error(400, 'Attach documents for this verification.');
      }
      final projection = {
        'id': 'mock-verification-${++_sequence}',
        'business_id': vendorId,
        'dimension': dim,
        'subject_type': body['subject_type'],
        'subject_ref': body['subject_ref'],
        'status': 'PENDING_REVIEW',
        'version': 1,
      };
      verifications.removeWhere(
        (v) => v['dimension'] == dim && v['subject_ref'] == body['subject_ref'],
      );
      verifications.add(projection);
      for (final d in documents) {
        if (refs.contains(d['id'])) d['status'] = 'SUBMITTED';
      }
      status = 'PENDING_REVIEW';
      return commit(projection, status: 202);
    }
    if (path.endsWith('/request-activation')) {
      if (readiness['ready'] == true) {
        status = 'ACTIVATED';
        if ([
          'DRAFT',
          'PENDING_VERIFICATION',
          'PENDING_REVIEW',
        ].contains(business['status'])) {
          business['status'] = 'ACTIVE';
          business['version']++;
        }
        for (final record in [...capabilities, ...branches]) {
          if (['DRAFT', 'PENDING_REVIEW'].contains(record['status'])) {
            record['status'] = 'ACTIVE';
            record['version'] = (record['version'] as int) + 1;
          }
        }
        for (final branch in branches) {
          for (final capability in branch['capabilities'] as List? ?? []) {
            if (capability['status'] == 'DRAFT') {
              capability['status'] = 'ACTIVE';
              capability['version'] = (capability['version'] as int) + 1;
            }
          }
        }
      } else if (status != 'ACTIVATED') {
        status = verifications.any((v) => v['status'] == 'REJECTED')
            ? 'ACTION_REQUIRED'
            : 'PENDING_REVIEW';
      }
      return commit(readiness);
    }
    if (path.endsWith('/onboarding/submit')) {
      status = 'PENDING_VERIFICATION';
      business['status'] = 'PENDING_VERIFICATION';
      return commit(overview);
    }
    if (path.endsWith('/businesses/$vendorId')) {
      if (!version(business)) {
        return error(
          409,
          'Business details have changed. Refresh before saving.',
        );
      }
      business.addAll({...body, 'version': business['version'] + 1});
      return commit(business);
    }
    for (final entry in {
      'branches': branches,
      'brands': brands,
      'capabilities': capabilities,
    }.entries) {
      if (path.endsWith('/${entry.key}')) {
        final item = {
          ...body,
          'id': 'mock-${entry.key}-${++_sequence}',
          'business_id': vendorId,
          'status': 'DRAFT',
          'version': 1,
        };
        entry.value.add(item);
        return commit(item, status: 201);
      }
      final matches = entry.value.where(
        (v) => path.endsWith('/${entry.key}/${v['id']}'),
      );
      if (matches.isNotEmpty) {
        final item = matches.first;
        if (!version(item)) {
          return error(409, 'This record has changed. Refresh before saving.');
        }
        item.addAll({...body, 'version': item['version'] + 1});
        return commit(item);
      }
    }
    return error(501, 'This operation is not available in this mock.');
  }

  bool consumeExportGrant(String id, String purpose) {
    if (!['crm.export.create', 'crm.export.download'].contains(purpose) ||
        _grants[id] != purpose ||
        _grantExpiry[id] == null ||
        !DateTime.now().isBefore(_grantExpiry[id]!)) {
      return false;
    }
    _grants.remove(id);
    _grantExpiry.remove(id);
    return true;
  }

  bool consumePastryGrant(String id) {
    final expires = _grantExpiry.remove(id);
    return _grants.remove(id) == 'pastry.high_value_quote_approval' &&
        expires != null &&
        DateTime.now().isBefore(expires);
  }

  bool consumeSettingsGrant(String id) {
    final expiry = _grantExpiry[id];
    if (_grants[id] != 'iam.high_risk_permission_change' ||
        expiry == null ||
        !DateTime.now().isBefore(expiry)) {
      return false;
    }
    _grants.remove(id);
    _grantExpiry.remove(id);
    return true;
  }

  bool consumeWithdrawalGrant(String id) {
    final expires = _grantExpiry.remove(id);
    return _grants.remove(id) == 'finance.withdrawal' &&
        expires != null &&
        DateTime.now().isBefore(expires);
  }

  void invalidateSecurityChallenges() {
    _grants.clear();
    _grantExpiry.clear();
    _challenge = null;
    _purpose = null;
    _challengeExpires = null;
  }

  http.StreamedResponse? redeem(http.BaseRequest request, List<int> bytes) {
    final ref = request.url.path.split('/')[5];
    final grant = _uploads[ref];
    if (grant == null) return null;
    String? header(String name) =>
        request.headers[name] ?? request.headers[name.toLowerCase()];
    if (_completedUploads.contains(ref) ||
        header('X-Upload-Token') != grant['upload_token'] ||
        header('X-Upload-Authorization-ID') != grant['upload_session_id'] ||
        header('Content-Type') !=
            (grant['allowed_mime_types'] as List).single ||
        header('X-Declared-Size') != '${bytes.length}' ||
        header('X-Declared-Checksum-SHA256') !=
            sha256.convert(bytes).toString() ||
        bytes.length > grant['max_bytes'] ||
        DateTime.now().isAfter(DateTime.parse(grant['expires_at']))) {
      return error(400, 'Invalid, expired or already used upload grant.');
    }
    _completedUploads.add(ref);
    return ok({
      'mediaObjectId': ref,
      'uploadStatus': 'COMPLETED',
      'safetyStatus': 'SAFE',
    });
  }

  static http.StreamedResponse ok(Object? data, {int status = 200}) => response(
    status,
    status == 204 ? '' : jsonEncode({'success': true, 'data': data}),
  );
  static http.StreamedResponse error(int status, String message) => response(
    status,
    jsonEncode({
      'success': false,
      'error': {
        'code': status == 409
            ? 'CONFLICT'
            : status == 403
            ? 'FORBIDDEN'
            : status == 401
            ? 'UNAUTHORIZED'
            : status == 503
            ? 'UNAVAILABLE'
            : 'VALIDATION_ERROR',
        'message': message,
      },
    }),
  );
  static http.StreamedResponse response(int status, String body) =>
      http.StreamedResponse(
        Stream.value(utf8.encode(body)),
        status,
        headers: {'content-type': 'application/json'},
      );
}

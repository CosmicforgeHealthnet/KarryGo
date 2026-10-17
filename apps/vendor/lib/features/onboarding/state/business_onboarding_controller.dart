import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_app.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/network/vendor_response.dart';

class BusinessOnboardingController extends ChangeNotifier {
  BusinessOnboardingController(this.app);
  final VendorApp app;
  JsonObject business = {}, overview = {}, readiness = {}, bank = {};
  List<JsonObject> branches = [],
      brands = [],
      capabilities = [],
      catalogue = [],
      documents = [],
      verifications = [];
  bool busy = false, loaded = false, _disposed = false;
  String? error;
  ApiException? saveError;
  final Map<String, VendorCommand> _commands = {};
  String get businessId => app.scope.vendorId!;
  JsonObject get representative => overview['representative'] is Map
      ? jsonObject(overview['representative'])
      : {};
  bool get needsBusiness => loaded && business.isEmpty;
  bool get activated =>
      business['status'] == 'ACTIVE' && overview['status'] == 'ACTIVATED';
  bool get submitted => [
    'PENDING_VERIFICATION',
    'PENDING_REVIEW',
    'IN_REVIEW',
  ].contains(overview['status']);
  bool get blocked => ['SUSPENDED', 'CLOSED'].contains(business['status']);
  List<JsonObject> get requirements => objects(readiness['requirements']);
  static List<JsonObject> objects(Object? data) {
    if (data is Map) data = data['items'];
    if (data == null) return [];
    if (data is! List) throw const FormatException('Expected a server list');
    return data.map(jsonObject).toList();
  }

  Future<VendorResponse> request(
    String method,
    String path, {
    JsonObject? body,
    Map<String, String> params = const {},
    Map<String, String> query = const {},
  }) {
    final api = path.contains('/onboarding')
        ? app.onboardingApi
        : app.businessApi;
    final fingerprint = jsonEncode([method, path, params, body]);
    final command = method == 'GET'
        ? null
        : _commands.putIfAbsent(fingerprint, VendorCommand.create);
    return api
        .request(
          method,
          path,
          body:
              body == null ||
                  (body.isEmpty &&
                      api.operation(method, path).requestSchema == null)
              ? null
              : VendorPayload(body),
          pathParameters: params,
          query: query,
          command: command,
        )
        .then((response) {
          _commands.remove(fingerprint);
          return response;
        });
  }

  Future<VendorResponse> all(String path, Map<String, String> params) async {
    final items = <JsonObject>[];
    int offset = 0;
    while (true) {
      final response = await request(
        'GET',
        path,
        params: params,
        query: {'limit': '100', 'offset': '$offset'},
      );
      final page = objects(response.data);
      items.addAll(page);
      final pagination = response.data is Map
          ? (response.data as Map)['pagination']
          : null;
      if (pagination is! Map || pagination['has_more'] != true) {
        return VendorResponse(
          statusCode: response.statusCode,
          headers: response.headers,
          data: items,
        );
      }
      if (page.isEmpty) {
        throw const FormatException('Invalid pagination');
      }
      offset += page.length;
    }
  }

  void beginNewUpload() {
    _commands.removeWhere(
      (fingerprint, _) =>
          fingerprint.contains('/api/v1/vendor/onboarding/document-uploads'),
    );
  }

  Future<JsonObject> optional(String path) async {
    try {
      final r = await request('GET', path);
      return r.data == null ? {} : r.object;
    } on ApiException catch (e) {
      if (e.statusCode == 404) return {};
      rethrow;
    }
  }

  Future<bool> loadBankProfile() => run(() async {
    final profile = await optional('/api/v1/vendor/onboarding/bank-profile');
    if (_disposed) return;
    bank = profile;
    loaded = true;
  });

  Future<void> load() => run(() async {
    VendorResponse businessResponse;
    try {
      businessResponse = await request(
        'GET',
        '/api/v1/vendor/businesses/{businessId}',
        params: {'businessId': businessId},
      );
    } on ApiException catch (e) {
      if (e.statusCode != 404) rethrow;
      business = {};
      overview = {};
      readiness = {};
      branches = [];
      brands = [];
      capabilities = [];
      documents = [];
      verifications = [];
      bank = {};
      loaded = true;
      return;
    }
    final results = await Future.wait([
      Future.value(businessResponse),
      request('GET', '/api/v1/vendor/onboarding'),
      request('GET', '/api/v1/vendor/onboarding/activation-readiness'),
      all('/api/v1/vendor/businesses/{businessId}/branches', {
        'businessId': businessId,
      }),
      all('/api/v1/vendor/businesses/{businessId}/brands', {
        'businessId': businessId,
      }),
      all('/api/v1/vendor/businesses/{businessId}/capabilities', {
        'businessId': businessId,
      }),
      request('GET', '/api/v1/vendor/capability-types'),
      request('GET', '/api/v1/vendor/onboarding/documents'),
      request('GET', '/api/v1/vendor/onboarding/verifications'),
    ]);
    final nextBank = await optional('/api/v1/vendor/onboarding/bank-profile');
    if (_disposed) return;
    business = results[0].object;
    overview = results[1].object;
    readiness = results[2].object;
    branches = objects(results[3].data);
    brands = objects(results[4].data);
    capabilities = objects(results[5].data);
    for (final branch in branches) {
      branch['capabilities'] = objects(
        (await request(
          'GET',
          '/api/v1/vendor/businesses/{businessId}/branches/{branchId}/capabilities',
          params: {'businessId': businessId, 'branchId': branch['id']},
        )).data,
      );
    }
    catalogue = objects(results[6].data);
    documents = objects(results[7].data);
    verifications = objects(results[8].data);
    bank = nextBank;
    loaded = true;
  });
  Future<bool> run(Future<void> Function() work) async {
    if (busy || _disposed) return false;
    busy = true;
    error = null;
    saveError = null;
    notifyListeners();
    try {
      await work();
      return !_disposed;
    } on ApiException catch (e) {
      saveError = e;
      error = e.fields.isEmpty
          ? e.message
          : e.fields.map((field) => field.message).toSet().join('\n');
      return false;
    } on FormatException {
      error =
          'The server returned an incomplete onboarding response. Refresh to try again.';
      return false;
    } catch (_) {
      error = 'This step could not be completed. Please try again.';
      return false;
    } finally {
      busy = false;
      if (!_disposed) notifyListeners();
    }
  }

  Future<bool> save(
    String method,
    String path,
    JsonObject body, {
    Map<String, String> params = const {},
  }) => run(() async {
    await request(method, path, body: body, params: params);
  });
  @override
  void dispose() {
    _disposed = true;
    _commands.clear();
    super.dispose();
  }
}

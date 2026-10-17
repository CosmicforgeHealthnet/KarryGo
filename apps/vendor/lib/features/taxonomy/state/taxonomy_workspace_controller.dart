import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';

class TaxonomyWorkspaceController extends ChangeNotifier {
  TaxonomyWorkspaceController(this.runtime, this.sourceType, this.sourceRef);
  final VendorRuntime runtime;
  final String sourceType, sourceRef;
  String get namespace => sourceType == 'FOOD_ITEM' ? 'FOOD' : 'SHOP';
  late final app = runtime.backend;
  late final storage = runtime.mode == VendorDataMode.mock
      ? runtime.mockSupportPreferences
      : runtime.preferences;
  late final recoveryKey =
      'vendor.taxonomy.${base64Url.encode(utf8.encode('${runtime.mode.name}|${runtime.mode == VendorDataMode.mock ? runtime.revision : app.client.config.origin}|${app.scope.vendorId}|${app.auth.session?.staffId}|$sourceType|$sourceRef'))}';
  List<JsonObject> facets = [],
      nodes = [],
      claims = [],
      mappings = [],
      tags = [],
      assignments = [];
  JsonObject? version, pending;
  bool loading = false,
      busy = false,
      stale = true,
      blocked = false,
      denied = false,
      disposed = false;
  String? error;
  bool get writable =>
      !loading && !busy && !stale && !blocked && !denied && pending == null;
  void emit() {
    if (!disposed) notifyListeners();
  }

  List<JsonObject> list(Object? value) {
    if (value != null && value is! List) {
      throw const FormatException('Invalid taxonomy list');
    }
    return (value as List? ?? []).map(jsonObject).toList();
  }

  Future<void> restore() async {
    try {
      final raw = await storage.read(recoveryKey);
      if (raw == null || raw == '{}') {
        pending = null;
        blocked = false;
        return;
      }
      final saved = jsonObject(jsonDecode(raw));
      final body = jsonObject(saved['body']);
      if (saved['key'] is! String ||
          body['source_type'] != sourceType ||
          body['source_ref'] != sourceRef ||
          body['facet_code'] is! String ||
          body['mappings'] is! List) {
        throw const FormatException('Invalid saved mapping');
      }
      pending = saved;
      blocked = false;
    } catch (_) {
      blocked = true;
      error =
          'Your saved change could not be restored. Reopen classification before editing.';
    }
  }

  Future<void> read() async {
    final api = app.taxonomyApi;
    final published =
        (await api.getTaxonomyNamespacesNamespaceCodePublishedVersion(
          namespaceCode: namespace,
        )).object;
    final nextVersion = jsonObject(published['version']);
    if (published['namespace_code'] != namespace ||
        nextVersion['ID'] is! String ||
        nextVersion['Status'] != 'PUBLISHED') {
      throw const FormatException('No valid published taxonomy');
    }
    final f = (await api.getTaxonomyNamespacesNamespaceCodeFacets(
      namespaceCode: namespace,
    )).object;
    final n = (await api.getTaxonomyNamespacesNamespaceCodeNodes(
      namespaceCode: namespace,
    )).object;
    if (f['version_id'] != nextVersion['ID'] ||
        n['version_id'] != nextVersion['ID']) {
      throw const FormatException('Taxonomy publication changed; refresh');
    }
    final nextFacets = list(f['facets']);
    final nextNodes = list(n['nodes']);
    if ([
      ...nextFacets,
      ...nextNodes,
    ].any((r) => r['VersionID'] != nextVersion['ID'] || r['ID'] is! String)) {
      throw const FormatException('Invalid published content');
    }
    final classification =
        (await api.getTaxonomyNamespacesNamespaceCodeClassifications(
          namespaceCode: namespace,
          query: {'source_type': sourceType, 'source_ref': sourceRef},
        )).object;
    if (classification['source_type'] != sourceType ||
        classification['source_ref'] != sourceRef ||
        classification['namespace_code'] != namespace ||
        classification['business_ref'] != app.scope.vendorId) {
      throw const FormatException('Out-of-scope classification');
    }
    final nextMappings = list(classification['mappings']);
    final nextTags = list(classification['tag_assignments']);
    final nextAssignments = list(classification['dietary_claim_assignments']);
    if (nextMappings.any(
          (r) =>
              r['SourceType'] != sourceType ||
              r['SourceRef'] != sourceRef ||
              r['BusinessRef'] != app.scope.vendorId,
        ) ||
        [...nextTags, ...nextAssignments].any(
          (r) =>
              r['source_type'] != sourceType ||
              r['source_ref'] != sourceRef ||
              r['business_ref'] != app.scope.vendorId,
        )) {
      throw const FormatException('Out-of-scope assignments');
    }
    final dietary = (await api.getTaxonomyNamespacesNamespaceCodeDietaryClaims(
      namespaceCode: namespace,
    )).object;
    version = nextVersion;
    facets = nextFacets;
    nodes = nextNodes;
    mappings = nextMappings;
    tags = nextTags;
    assignments = nextAssignments;
    claims = list(dietary['dietary_claims']);
  }

  Future<void> load() async {
    if (loading || busy || disposed) return;
    loading = true;
    stale = true;
    error = null;
    emit();
    try {
      await restore();
      await read();
      stale = false;
    } catch (e) {
      error = e is ApiException && e.statusCode == 404
          ? 'Karryam has not set up categories for this yet. Your item can still be sold. Check back later to classify it.'
          : e is ApiException && e.statusCode != null && e.statusCode! < 500
          ? e.message
          : 'Classification could not be loaded. Check your connection and refresh.';
    } finally {
      loading = false;
      emit();
    }
  }

  List<JsonObject> activeMappings(String facetId) => mappings
      .where(
        (m) =>
            m['Status'] == 'ACTIVE' &&
            nodes.any((n) => n['ID'] == m['NodeID'] && n['FacetID'] == facetId),
      )
      .toList();
  bool get unresolvedMappings => mappings.any(
    (m) => m['Status'] == 'ACTIVE' && !nodes.any((n) => n['ID'] == m['NodeID']),
  );
  String reviewToken() =>
      jsonEncode({'version': version, 'mappings': mappings});
  Future<bool> save({
    String? facetCode,
    List<JsonObject>? candidates,
    String? reviewed,
  }) async {
    if (busy ||
        loading ||
        blocked ||
        denied ||
        disposed ||
        pending == null && !writable) {
      return false;
    }
    if (pending == null) {
      final facet = facets
          .where((f) => f['Code'] == facetCode && f['Status'] == 'ACTIVE')
          .firstOrNull;
      if (facet == null ||
          candidates == null ||
          unresolvedMappings ||
          candidates.where((m) => m['mapping_type'] == 'PRIMARY').length > 1 ||
          candidates.map((m) => m['node_code']).toSet().length !=
              candidates.length ||
          candidates.any(
            (m) =>
                ![
                  'PRIMARY',
                  'SECONDARY',
                  'DISCOVERY',
                ].contains(m['mapping_type']) ||
                !nodes.any(
                  (n) =>
                      n['FacetID'] == facet['ID'] &&
                      n['CanonicalCode'] == m['node_code'] &&
                      n['Status'] == 'ACTIVE',
                ),
          )) {
        error = 'Choose valid categories with at most one primary category.';
        emit();
        return false;
      }
    }
    busy = true;
    error = null;
    emit();
    try {
      if (pending == null) {
        await read();
        if (reviewed != reviewToken()) {
          stale = false;
          error =
              'Classification changed. Review the latest categories before saving.';
          return false;
        }
        final saved = {
          'key': VendorCommand.create().idempotencyKey,
          'body': {
            'facet_code': facetCode,
            'source_type': sourceType,
            'source_ref': sourceRef,
            'mappings': candidates,
          },
        };
        final frozen = jsonObject(jsonDecode(jsonEncode(saved)));
        await storage.write(recoveryKey, jsonEncode(frozen));
        pending = frozen;
      }
      final result = (await app.taxonomyApi.request(
        'PUT',
        '/api/v1/vendor/taxonomy/namespaces/{namespaceCode}/mappings',
        pathParameters: {'namespaceCode': namespace},
        body: VendorPayload(jsonObject(pending!['body'])),
        command: VendorCommand(idempotencyKey: pending!['key']),
      )).object;
      if (result['namespace_code'] != namespace ||
          result['source_type'] != sourceType ||
          result['source_ref'] != sourceRef ||
          result['facet_code'] != pending!['body']['facet_code'] ||
          result['mapping_ids'] is! List) {
        throw const FormatException('Invalid confirmation');
      }
      await storage.write(recoveryKey, '{}');
      pending = null;
      try {
        await read();
        stale = false;
      } catch (_) {
        stale = true;
        error = 'Saved. Refresh to see the latest classification.';
      }
      return true;
    } catch (e) {
      stale = true;
      error = e is ApiException
          ? e.message
          : pending == null
          ? 'Nothing was submitted. Your change could not be saved.'
          : 'The result is unconfirmed. Retry the saved change.';
      if (e is ApiException &&
          [400, 401, 403, 404, 409, 422].contains(e.statusCode)) {
        if (e.statusCode == 403) denied = true;
        try {
          await storage.write(recoveryKey, '{}');
          pending = null;
        } catch (_) {
          blocked = true;
        }
        stale = true;
      }
      return false;
    } finally {
      busy = false;
      emit();
    }
  }

  Future<JsonObject> nodeDetail(String id) async =>
      (await app.taxonomyApi.getTaxonomyNamespacesNamespaceCodeNodesNodeId(
        namespaceCode: namespace,
        nodeId: id,
      )).object;
  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }
}

import 'dart:convert';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import 'crm_workspace_controller.dart';

const exportTypes = {
  'CUSTOMER_SUMMARY': 'Customer summary',
  'SEGMENT_MEMBERS': 'Segment members',
  'CORPORATE_CUSTOMERS': 'Corporate accounts',
  'LOYALTY_SUMMARY': 'Loyalty summary',
  'CAMPAIGN_PERFORMANCE_SUMMARY': 'Campaign performance',
  'INTELLIGENCE_SCORES': 'Customer intelligence',
};
const exportStates = [
  'QUEUED',
  'RUNNING',
  'READY',
  'FAILED',
  'CANCELLED',
  'EXPIRED',
];

class CrmExportController extends CrmWorkspaceController {
  CrmExportController(VendorRuntime runtime, {String? id})
    : super(
        runtime,
        id == null ? '$crmBase/exports' : '$crmBase/exports/{exportId}',
        parameters: id == null ? const {} : {'exportId': id},
        wrapper: id == null ? 'exports' : null,
      );
  @override
  void validate(JsonObject r, {bool checkBranch = true}) {
    super.validate(r, checkBranch: checkBranch);
    if (r['export_ref'] is! String ||
        !exportTypes.containsKey(r['export_type']) ||
        !exportStates.contains(r['status']) ||
        !['CSV', 'JSONL'].contains(r['format'])) {
      throw const FormatException('Invalid export response');
    }
  }

  bool get pendingCreation => pending?['path'] == '$crmBase/exports';
  Future<JsonObject?> createExport(String grant, {JsonObject? settings}) async {
    if (busy ||
        loading ||
        blocked ||
        disposed ||
        grant.isEmpty ||
        pending != null && (!pendingCreation || settings != null) ||
        pending == null && !writable) {
      return null;
    }
    busy = true;
    error = null;
    emit();
    try {
      if (pending == null) {
        if (settings == null || settings.containsKey('step_up_grant_id')) {
          throw const VendorContractException('Invalid export settings.');
        }
        final body = jsonObject(jsonDecode(jsonEncode(settings)));
        validateRequest({
          ...body,
          'step_up_grant_id': grant,
        }, app.crmApi.operation('POST', '$crmBase/exports').requestSchema!);
        final saved = {
          'method': 'POST',
          'path': '$crmBase/exports',
          'params': <String, String>{},
          'body': body,
          'key': VendorCommand.create().idempotencyKey,
        };
        await storage.write(recoveryKey, jsonEncode(saved));
        pending = saved;
      }
      final row = (await app.crmApi.request(
        'POST',
        '$crmBase/exports',
        body: VendorPayload({
          ...jsonObject(pending!['body']),
          'step_up_grant_id': grant,
        }),
        command: VendorCommand(idempotencyKey: pending!['key']),
      )).object;
      if (row['export_ref'] is! String ||
          row['version'] is! int ||
          !exportStates.contains(row['status'])) {
        throw const FormatException('Missing export confirmation');
      }
      await storage.write(recoveryKey, '{}');
      pending = null;
      return row;
    } catch (e) {
      error = e is ApiException
          ? e.message
          : pending != null
          ? 'The outcome is unconfirmed. Verify and retry the saved export.'
          : 'Nothing was submitted. Check the export settings.';
      if (e is ApiException &&
          [400, 401, 403, 404, 409, 412, 422].contains(e.statusCode)) {
        try {
          await storage.write(recoveryKey, '{}');
          pending = null;
          stale = true;
        } catch (_) {
          blocked = true;
        }
      }
      return null;
    } finally {
      busy = false;
      emit();
    }
  }
}

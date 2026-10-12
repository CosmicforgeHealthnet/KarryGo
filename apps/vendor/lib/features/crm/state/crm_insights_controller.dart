import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import 'crm_workspace_controller.dart';

class CrmInsightsController extends CrmWorkspaceController {
  CrmInsightsController(VendorRuntime runtime, {String? model})
    : super(
        runtime,
        '$crmBase/intelligence/${model == null
            ? 'overview'
            : model == 'RFM_V1'
            ? 'rfm'
            : 'churn'}',
      );
  static const models = ['RFM_V1', 'CHURN_V1'];
  static const states = [
    'NOT_AVAILABLE',
    'QUEUED',
    'RUNNING',
    'CURRENT',
    'STALE',
    'FAILED',
  ];
  void model(JsonObject row) {
    if (!models.contains(row['model_key']) || !states.contains(row['status'])) {
      throw const FormatException('Invalid insights model');
    }
    for (final field in [
      'profile_count',
      'available_count',
      'unavailable_count',
    ]) {
      if (row[field] is! int || row[field] < 0) {
        throw const FormatException('Invalid insights coverage');
      }
    }
  }

  @override
  void validate(JsonObject r, {bool checkBranch = true}) {
    super.validate(r, checkBranch: checkBranch);
    if (r['evaluation_ref'] != null) {
      if (r['evaluation_ref'] is! String ||
          !models.contains(r['model_key']) ||
          ![
            'QUEUED',
            'RUNNING',
            'COMPLETED',
            'FAILED',
            'DEAD_LETTER',
            'SUPERSEDED',
          ].contains(r['status'])) {
        throw const FormatException('Invalid rebuild confirmation');
      }
      return;
    }
    if (r['models'] is List) {
      if (!['BUSINESS', 'BRANCH'].contains(r['scope']) ||
          r['branch_ref'] != readQuery['branch_ref']) {
        throw const FormatException('Invalid insights scope');
      }
      for (final item in r['models']) {
        model(jsonObject(item));
      }
    } else {
      model(jsonObject(r['availability']));
      for (final field in [
        'composite_distribution',
        'recency_distribution',
        'frequency_distribution',
        'monetary_distribution',
        'band_distribution',
        'unavailable_reasons',
      ]) {
        if (r.containsKey(field) && r[field] != null) {
          if (r[field] is! List) {
            throw const FormatException('Invalid insights distribution');
          }
          for (final value in r[field]) {
            final bucket = jsonObject(value);
            if (bucket['key'] is! String ||
                bucket['count'] is! int ||
                bucket['count'] < 0) {
              throw const FormatException('Invalid insights bucket');
            }
          }
        }
      }
    }
  }
}

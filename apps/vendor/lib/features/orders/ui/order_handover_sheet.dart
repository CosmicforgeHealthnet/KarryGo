import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';
import '../../dashboard/ui/dashboard_empty_state.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../handover/models/handover_models.dart';

class OrderHandoverSheet extends StatefulWidget {
  const OrderHandoverSheet({
    super.key,
    required this.runtime,
    required this.orderId,
    required this.missionId,
  });
  final VendorRuntime runtime;
  final String orderId, missionId;
  @override
  State<OrderHandoverSheet> createState() => _OrderHandoverSheetState();
}

class _OrderHandoverSheetState extends State<OrderHandoverSheet> {
  final _credential = TextEditingController();
  final _form = GlobalKey<FormState>();
  JsonObject? _observation;
  String? _error;
  bool _busy = false, _restricted = false, _unknown = false;
  String _method = 'OTP';
  @override
  void initState() {
    super.initState();
    if (widget.runtime.mode == VendorDataMode.mock) _credential.text = '654321';
    _read();
  }

  @override
  void dispose() {
    _credential.clear();
    _credential.dispose();
    super.dispose();
  }

  Future<void> _read() async {
    setState(() {
      _busy = true;
      _error = null;
      _restricted = false;
    });
    try {
      final response = await widget.runtime.backend.handoverApi
          .getCommerceOrdersVendorOrderIdMissionsMissionIdHandoverStage(
            vendorOrderId: widget.orderId,
            missionId: widget.missionId,
            stage: 'PICKUP',
          );
      if (!mounted) return;
      _observation = response.object;
      _unknown = false;
    } catch (e) {
      if (!mounted) return;
      _error = e is ApiException
          ? e.message
          : 'Pickup confirmation could not be loaded.';
      _restricted = e is ApiException && e.statusCode == 403;
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _confirm() async {
    if (!_form.currentState!.validate() || _busy || _unknown) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      final response = await widget.runtime.backend.handoverApi
          .postCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirm(
            vendorOrderId: widget.orderId,
            missionId: widget.missionId,
            command: VendorCommand.create(),
            body:
                PostCommerceOrdersVendorOrderIdMissionsMissionIdHandoverPickupConfirmRequest.fromJson(
                  {
                    'handover_ref': _observation!['handover_ref'],
                    'method': _method,
                    'credential': _credential.text.trim(),
                  },
                ),
          );
      if (!mounted) return;
      _observation = response.object;
      _credential.clear();
      if (_observation!['status'] == 'CONSUMED') {
        Navigator.pop(context, true);
      }
    } catch (e) {
      if (!mounted) return;
      _unknown =
          e is! ApiException ||
          e.statusCode == null ||
          (e.statusCode ?? 0) >= 500;
      _error = _unknown
          ? 'Confirmation could not be verified. Refresh the handover status before trying again.'
          : e is ApiException
          ? e.message
          : 'Verification could not be completed.';
      _restricted = e is ApiException && e.statusCode == 403;
      _credential.clear();
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final confirmed = _observation?['status'] == 'CONSUMED';
    final direction = _observation?['direction'];
    final verifier = direction is Map ? direction['verifier_role'] : null;
    return PopScope(
      canPop: !_busy,
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.sizeOf(context).height * .85,
        ),
        child: Padding(
          padding: EdgeInsets.only(
            bottom: MediaQuery.viewInsetsOf(context).bottom,
          ),
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
            child: Form(
              key: _form,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Icon(
                    Icons.verified_user_outlined,
                    size: 34,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                  const SizedBox(height: 14),
                  const Text(
                    'Verify pickup',
                    style: TextStyle(fontSize: 25, fontWeight: FontWeight.w700),
                  ),
                  const SizedBox(height: 10),
                  const Text(
                    'Check the pickup credential presented by the delivery provider before handing over the goods.',
                  ),
                  const SizedBox(height: 20),
                  if (_error != null)
                    DashboardEmptyState(
                      story: _restricted
                          ? EmptyStory.restricted
                          : EmptyStory.unavailable,
                      title: 'Verification unavailable',
                      message: _error!,
                      action: 'Refresh handover',
                      onAction: _busy ? null : _read,
                    ),
                  if (_observation != null) ...[
                    Text('Status: ${dashboardLabel(_observation!['status'])}'),
                    const SizedBox(height: 16),
                    if (!confirmed &&
                        _observation?['status'] == 'ACTIVE' &&
                        verifier == 'VENDOR_SENDER' &&
                        !_restricted) ...[
                      DropdownButtonFormField<String>(
                        initialValue: _method,
                        decoration: const InputDecoration(
                          labelText: 'Credential method',
                        ),
                        items: const [
                          DropdownMenuItem(
                            value: 'OTP',
                            child: Text('Pickup code'),
                          ),
                          DropdownMenuItem(
                            value: 'QR',
                            child: Text('QR credential'),
                          ),
                        ],
                        onChanged: _busy
                            ? null
                            : (value) => setState(() {
                                _method = value!;
                                _credential.clear();
                              }),
                      ),
                      const SizedBox(height: 16),
                      TextFormField(
                        controller: _credential,
                        enabled: !_busy,
                        obscureText: true,
                        enableSuggestions: false,
                        autocorrect: false,
                        decoration: InputDecoration(
                          labelText: _method == 'OTP'
                              ? 'Pickup code'
                              : 'QR credential',
                        ),
                        validator: (value) =>
                            value == null || value.trim().isEmpty
                            ? 'Enter the pickup credential'
                            : null,
                      ),
                      const SizedBox(height: 20),
                      PrimaryAction(
                        label: 'Verify pickup',
                        busy: _busy,
                        onPressed: _unknown ? null : _confirm,
                      ),
                    ] else if (confirmed)
                      PrimaryAction(
                        label: 'Done',
                        onPressed: () => Navigator.pop(context, true),
                      ),
                  ] else if (_busy)
                    Center(
                      child: CircularProgressIndicator(
                        color: KarryamColors.tealInk.forInk(context),
                      ),
                    ),
                  if (!_busy)
                    Center(
                      child: TextButton(
                        onPressed: () => Navigator.pop(context),
                        child: const Text('Close'),
                      ),
                    ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

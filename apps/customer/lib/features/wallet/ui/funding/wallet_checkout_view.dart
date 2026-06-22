import 'package:flutter/material.dart';
import 'package:webview_flutter/webview_flutter.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';
import '../../state/wallet_funding_controller.dart';
import '../widgets/wallet_flow_scaffold.dart';

/// Hosts the Paystack checkout in an in-app WebView. When the WebView navigates
/// to the payment callback (or a Paystack completion URL), it tells the
/// controller to verify the wallet credit.
class WalletCheckoutView extends StatefulWidget {
  const WalletCheckoutView({super.key, required this.controller});

  final WalletFundingController controller;

  @override
  State<WalletCheckoutView> createState() => _WalletCheckoutViewState();
}

class _WalletCheckoutViewState extends State<WalletCheckoutView> {
  late final WebViewController _webController;
  bool _loading = true;
  bool _completed = false;
  String? _loadError;

  @override
  void initState() {
    super.initState();
    final url = widget.controller.topUp!.authorizationUrl;
    _webController = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setBackgroundColor(Colors.white)
      ..setNavigationDelegate(
        NavigationDelegate(
          onPageStarted: (_) {
            if (mounted) setState(() => _loading = true);
          },
          onPageFinished: (_) {
            if (mounted) setState(() => _loading = false);
          },
          onProgress: (progress) {
            // Some pages never fire onPageFinished cleanly; clear the spinner
            // once content is essentially loaded.
            if (progress >= 90 && mounted && _loading) {
              setState(() => _loading = false);
            }
          },
          onWebResourceError: (error) {
            // Only surface errors for the main document, not sub-resources.
            if (error.isForMainFrame == true && mounted) {
              setState(() {
                _loading = false;
                _loadError =
                    'Could not load the payment page. Check your connection and try again.';
              });
            }
          },
          onNavigationRequest: (request) {
            if (_isReturnUrl(request.url)) {
              _handleCompletion();
              return NavigationDecision.prevent;
            }
            return NavigationDecision.navigate;
          },
        ),
      )
      ..loadRequest(Uri.parse(url));
  }

  /// Paystack redirects to the merchant callback URL (our `/topups/...` path,
  /// with `?trxref=...&reference=...`) on completion, and to a close URL on
  /// cancel. The initial checkout URL is `checkout.paystack.com/<code>`, which
  /// matches none of these.
  bool _isReturnUrl(String url) {
    final lower = url.toLowerCase();
    return lower.contains('/topups/') ||
        lower.contains('trxref=') ||
        lower.contains('reference=') ||
        lower.contains('paystack.co/close') ||
        lower.contains('/standard/success');
  }

  void _handleCompletion() {
    if (_completed) return;
    _completed = true;
    widget.controller.onCheckoutReturned();
  }

  void _retry() {
    setState(() {
      _loadError = null;
      _loading = true;
    });
    _webController.reload();
  }

  @override
  Widget build(BuildContext context) {
    return WalletFlowScaffold(
      title: 'Complete Payment',
      onBack: widget.controller.backToAmount,
      body: Stack(
        children: [
          if (_loadError == null) WebViewWidget(controller: _webController),
          if (_loading && _loadError == null)
            const ColoredBox(
              color: Colors.white,
              child: Center(
                child: CircularProgressIndicator(
                  color: CustomerFigmaColors.primary,
                ),
              ),
            ),
          if (_loadError != null)
            Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.wifi_off_rounded,
                        color: CustomerFigmaColors.muted, size: 40),
                    const SizedBox(height: 16),
                    Text(
                      _loadError!,
                      textAlign: TextAlign.center,
                      style: const TextStyle(
                        color: CustomerFigmaColors.text,
                        fontSize: 14,
                      ),
                    ),
                    const SizedBox(height: 20),
                    FigmaPrimaryButton(label: 'Try again', onPressed: _retry),
                    const SizedBox(height: 10),
                    FigmaSecondaryButton(
                      label: 'Back',
                      onPressed: widget.controller.backToAmount,
                    ),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }
}

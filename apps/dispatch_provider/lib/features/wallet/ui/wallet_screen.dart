import 'package:flutter/material.dart';
import 'earning_summary_screen.dart';
import 'log_dispute_screen.dart';
import 'withdrawal_form_screen.dart';
import '../state/wallet_controller.dart';


// ── Card clipper with bottom-right notch ──────────────────────────────────────

class _CardWithNotchClipper extends CustomClipper<Path> {
  const _CardWithNotchClipper({
    required this.radius,
    required this.notchWidth,
    required this.notchHeight,
    required this.notchRadius,
  });

  final double radius;
  final double notchWidth;
  final double notchHeight;
  final double notchRadius;

  @override
  Path getClip(Size size) {
    final path = Path();
    final w = size.width;
    final h = size.height;
    final r = radius;
    final nw = notchWidth;
    final nh = notchHeight;
    final nr = notchRadius;

    path.moveTo(r, 0);
    // top edge
    path.lineTo(w - r, 0);
    path.arcToPoint(Offset(w, r), radius: Radius.circular(r));
    // right edge down to notch start
    path.lineTo(w, h - nh - nr);
    // notch top-right concave curve
    path.arcToPoint(
      Offset(w - nr, h - nh),
      radius: Radius.circular(nr),
      clockwise: false,
    );
    // notch bottom edge (goes left)
    path.lineTo(w - nw + nr, h - nh);
    // notch bottom-left concave curve
    path.arcToPoint(
      Offset(w - nw, h - nh + nr),
      radius: Radius.circular(nr),
      clockwise: false,
    );
    // down to bottom-right of left portion
    path.lineTo(w - nw, h - r);
    path.arcToPoint(Offset(w - nw - r, h), radius: Radius.circular(r));
    // bottom edge going left
    path.lineTo(r, h);
    // bottom-left corner
    path.arcToPoint(Offset(0, h - r), radius: Radius.circular(r));
    // left edge
    path.lineTo(0, r);
    // top-left corner
    path.arcToPoint(Offset(r, 0), radius: Radius.circular(r));
    path.close();
    return path;
  }

  @override
  bool shouldReclip(covariant CustomClipper<Path> oldClipper) => false;
}

// ── Screen ────────────────────────────────────────────────────────────────────

class WalletScreen extends StatefulWidget {
  final WalletController walletController;

  const WalletScreen({super.key, required this.walletController});

  @override
  State<WalletScreen> createState() => _WalletScreenState();
}

class _WalletScreenState extends State<WalletScreen> {
  bool _balanceVisible = true;
  bool _navigating = false;

  WalletController get _controller => widget.walletController;

  @override
  void initState() {
    super.initState();
    _controller.addListener(_onControllerUpdate);
    _controller.loadEarnings();
  }

  @override
  void dispose() {
    _controller.removeListener(_onControllerUpdate);
    super.dispose();
  }

  void _onControllerUpdate() {
    if (mounted) setState(() {});
  }

  static const double _btnHeight = 44.0;
  static const double _btnWidth = 130.0;
  static const double _notchRadius = 16.0;
  static const double _cardRadius = 20.0;

  void _goToWithdraw() {
    if (_navigating) return;
    _navigating = true;
    Navigator.of(context)
        .push(MaterialPageRoute(
          builder: (_) => WithdrawalFormScreen(walletController: _controller),
        ))
        .then((_) => _navigating = false);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: CustomScrollView(
          slivers: [
            // ── Header ──────────────────────────────────────────────────
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 20, 20, 16),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    const Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Earnings',
                          style: TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w800,
                            color: Color(0xFF1A1A1A),
                          ),
                        ),
                        SizedBox(height: 2),
                        Text(
                          'Manage your Income here.',
                          style: TextStyle(
                            fontSize: 13,
                            color: Color(0xFF888888),
                          ),
                        ),
                      ],
                    ),
                    GestureDetector(
                      onTap: () => Navigator.of(context).push(
                        MaterialPageRoute(
                          builder: (_) => EarningSummaryScreen(walletController: _controller),
                        ),
                      ),
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 10,
                        ),
                        decoration: BoxDecoration(
                          color: const Color(0xFF4CAF50),
                          borderRadius: BorderRadius.circular(999),
                        ),
                        child: const Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(
                              Icons.attach_money,
                              size: 16,
                              color: Colors.white,
                            ),
                            SizedBox(width: 4),
                            Text(
                              'Earning Summary',
                              style: TextStyle(
                                fontSize: 13,
                                fontWeight: FontWeight.w700,
                                color: Colors.white,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),

            // ── Balance card with notch ──────────────────────────────────
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                // Stack sized to the full card bounding box (before clipping)
                // so Positioned(bottom:0, right:0) lands exactly at card corner
                child: Stack(
                  clipBehavior: Clip.none,
                  children: [
                    // clipped green card
                    ClipPath(
                      clipper: const _CardWithNotchClipper(
                        radius: _cardRadius,
                        notchWidth: _btnWidth,
                        notchHeight: _btnHeight,
                        notchRadius: _notchRadius,
                      ),
                      child: Container(
                        width: double.infinity,
                        padding: const EdgeInsets.fromLTRB(
                          20,
                          18,
                          20,
                          _btnHeight,
                        ),
                        decoration: const BoxDecoration(
                          gradient: LinearGradient(
                            colors: [Color(0xFF1B5E20), Color(0xFF388E3C)],
                            begin: Alignment.topLeft,
                            end: Alignment.bottomRight,
                          ),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      const Text(
                                        'Available Balance',
                                        style: TextStyle(
                                          fontSize: 13,
                                          color: Colors.white70,
                                          fontWeight: FontWeight.w500,
                                        ),
                                      ),
                                      const SizedBox(height: 4),
                                      Text(
                                        _balanceVisible
                                            ? (_controller.earnings != null
                                                ? _controller.earnings!.availableNaira
                                                : (_controller.isLoading ? '₦ ---' : '₦ 0.00'))
                                            : '₦ ****',
                                        style: const TextStyle(
                                          fontSize: 30,
                                          fontWeight: FontWeight.w800,
                                          color: Colors.white,
                                          letterSpacing: -0.5,
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                                Column(
                                  crossAxisAlignment: CrossAxisAlignment.center,
                                  children: [
                                    GestureDetector(
                                      onTap: () => setState(
                                        () =>
                                            _balanceVisible = !_balanceVisible,
                                      ),
                                      child: Icon(
                                        _balanceVisible
                                            ? Icons.visibility_outlined
                                            : Icons.visibility_off_outlined,
                                        color: Colors.white70,
                                        size: 22,
                                      ),
                                    ),
                                    const SizedBox(height: 12),
                                    GestureDetector(
                                      onTap: () => Navigator.of(context).push(
                                        MaterialPageRoute(
                                          builder: (_) =>
                                              const LogDisputeScreen(),
                                        ),
                                      ),
                                      child: Column(
                                        mainAxisSize: MainAxisSize.min,
                                        children: [
                                          Container(
                                            width: 46,
                                            height: 46,
                                            decoration: BoxDecoration(
                                              color: Colors.white.withValues(
                                                alpha: 0.18,
                                              ),
                                              shape: BoxShape.circle,
                                            ),
                                            child: const Icon(
                                              Icons.receipt_long_outlined,
                                              size: 22,
                                              color: Colors.white,
                                            ),
                                          ),
                                          const SizedBox(height: 5),
                                          const Text(
                                            'Dispute',
                                            style: TextStyle(
                                              fontSize: 11,
                                              color: Colors.white70,
                                              fontWeight: FontWeight.w500,
                                            ),
                                          ),
                                        ],
                                      ),
                                    ),
                                  ],
                                ),
                              ],
                            ),
                            const SizedBox(height: 20),
                            // bottom stats row
                            Row(
                              children: [
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      const Text(
                                        'Pending Balance',
                                        style: TextStyle(
                                          fontSize: 11,
                                          color: Colors.white60,
                                        ),
                                      ),
                                      const SizedBox(height: 3),
                                      Text(
                                        _balanceVisible
                                            ? (_controller.earnings != null
                                                ? _controller.earnings!.pendingNaira
                                                : '₦ 0.00')
                                            : '₦ ****',
                                        style: const TextStyle(
                                          fontSize: 13,
                                          fontWeight: FontWeight.w700,
                                          color: Colors.white,
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      const Text(
                                        "Today's Earnings",
                                        style: TextStyle(
                                          fontSize: 11,
                                          color: Colors.white60,
                                        ),
                                      ),
                                      const SizedBox(height: 3),
                                      Text(
                                        _balanceVisible ? '₦ 0.00' : '₦ ****',
                                        style: const TextStyle(
                                          fontSize: 13,
                                          fontWeight: FontWeight.w700,
                                          color: Colors.white,
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                                const SizedBox(width: _btnWidth),
                              ],
                            ),
                          ],
                        ),
                      ),
                    ),

                    Positioned(
                      bottom: 0,
                      right: 0,
                      width: _btnWidth,
                      height: _btnHeight,
                      child: GestureDetector(
                        behavior: HitTestBehavior.opaque,
                        onTap: _goToWithdraw,
                        child: Container(
                          decoration: BoxDecoration(
                            color: Colors.white,
                            borderRadius: const BorderRadius.only(
                              topLeft: Radius.circular(_notchRadius),
                              bottomRight: Radius.circular(_cardRadius),
                            ),
                            border: Border.all(
                              color: const Color(0xFF4CAF50),
                              width: 1.5,
                            ),
                          ),
                          alignment: Alignment.center,
                          child: const Text(
                            'Withdraw',
                            style: TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w700,
                              color: Color(0xFF4CAF50),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),

            const SliverToBoxAdapter(child: SizedBox(height: 16)),

            // ── Stats row (backend gap: trips_today / hours_online not available) ──
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 16),
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 20,
                    vertical: 16,
                  ),
                  decoration: BoxDecoration(
                    color: Colors.white,
                    borderRadius: BorderRadius.circular(16),
                    border: Border.all(color: const Color(0xFFEEEEEE)),
                  ),
                  child: const Row(
                    children: [
                      Expanded(
                        child: Column(
                          children: [
                            Text(
                              'Trips Completed Today',
                              style: TextStyle(
                                fontSize: 12,
                                color: Color(0xFF4CAF50),
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            SizedBox(height: 6),
                            Text(
                              '—',
                              style: TextStyle(
                                fontSize: 22,
                                fontWeight: FontWeight.w800,
                                color: Color(0xFF888888),
                              ),
                            ),
                          ],
                        ),
                      ),
                      VerticalDivider(color: Color(0xFFEEEEEE)),
                      Expanded(
                        child: Column(
                          children: [
                            Text(
                              'Hours Online',
                              style: TextStyle(
                                fontSize: 12,
                                color: Color(0xFF4CAF50),
                                fontWeight: FontWeight.w600,
                              ),
                            ),
                            SizedBox(height: 6),
                            Text(
                              '—',
                              style: TextStyle(
                                fontSize: 22,
                                fontWeight: FontWeight.w800,
                                color: Color(0xFF888888),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),

            const SliverToBoxAdapter(child: SizedBox(height: 24)),

            // ── Recent Transactions header ────────────────────────────────
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    const Text(
                      'Recent Transactions',
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    Text(
                      'View All',
                      style: TextStyle(
                        fontSize: 13,
                        fontWeight: FontWeight.w600,
                        color: Colors.green.shade600,
                      ),
                    ),
                  ],
                ),
              ),
            ),

            const SliverToBoxAdapter(child: SizedBox(height: 12)),

            // Backend gap: no provider transaction history endpoint yet.
            // Show empty state instead of fake data.
            const SliverToBoxAdapter(
              child: Padding(
                padding: EdgeInsets.symmetric(horizontal: 20, vertical: 32),
                child: Column(
                  children: [
                    Icon(
                      Icons.receipt_long_outlined,
                      size: 48,
                      color: Color(0xFFCCCCCC),
                    ),
                    SizedBox(height: 12),
                    Text(
                      'No transactions yet',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w700,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                    SizedBox(height: 6),
                    Text(
                      'Complete a trip to start earning.',
                      style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                      textAlign: TextAlign.center,
                    ),
                  ],
                ),
              ),
            ),

            const SliverToBoxAdapter(child: SizedBox(height: 100)),
          ],
        ),
      ),
    );
  }
}

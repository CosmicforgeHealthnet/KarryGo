import PartnerCard from "@/components/PartnerCard";

export default function Vendors() {
  return (
    <section
      id="vendors"
      className="relative scroll-mt-20 bg-white pb-10 lg:pb-14"
    >
      <div className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 grid gap-5 sm:gap-6 lg:grid-cols-2">
        <PartnerCard
          tone="light"
          eyebrow="For Vendors"
          title={
            <>
              Sell more.
              <br />
              Reach further.
            </>
          }
          text="Put your restaurant, store or business in front of everyone moving through Karryam."
          points={[
            "Orders & inventory",
            "Delivery handled for you",
            "Payouts & wallet",
            "Sales insights",
          ]}
          cta="Join as a Vendor"
          href="/waitlist"
          image="/images/fleet.png"
          alt="Karryam vendor dashboard showing orders and sales"
          imageWidth={1200}
          imageHeight={1000}
        />

        <PartnerCard
          tone="dark"
          eyebrow="API Platform"
          title={
            <>
              Integrate. Extend.
              <br />
              Create new possibilities.
            </>
          }
          text="Build on Karryam with secure endpoints, webhooks and clear documentation."
          cta="Request API access"
          href="/waitlist"
          backgroundImage="/images/api-skyline.png"
        />
      </div>
    </section>
  );
}
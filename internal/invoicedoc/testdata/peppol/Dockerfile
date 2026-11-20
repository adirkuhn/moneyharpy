# A Peppol BIS Billing 3.0 validator for the tests: Saxon running the
# official Peppol and EN 16931 Schematron rules, compiled to XSLT with
# SchXslt. The rules are fetched, not kept in the repository; every
# download is pinned and checked.
FROM eclipse-temurin:21-jre@sha256:ca73b4d3961703d6d7830a4361f49ef0478f8f28b39defc07bb72f4cc7f834f6

WORKDIR /validator
RUN set -e; \
    M=https://repo1.maven.org/maven2; \
    R=https://raw.githubusercontent.com/OpenPEPPOL/peppol-bis-invoice-3/3.0.22/rules/sch; \
    mkdir lib rules; \
    fetch() { curl -sfL -o "$2" "$1"; echo "$3  $2" | sha256sum -c -; }; \
    fetch $M/net/sf/saxon/Saxon-HE/12.7/Saxon-HE-12.7.jar lib/saxon.jar c607e8b395d4ee237495ad163183971ca3453d491534c847ebb55e0462772086; \
    fetch $M/org/xmlresolver/xmlresolver/5.3.3/xmlresolver-5.3.3.jar lib/xmlresolver.jar 1fe4d5b92f708dcdb82dbce12919e0171e6b5ca62c6dca6220483625098feb5f; \
    fetch $M/org/xmlresolver/xmlresolver/5.3.3/xmlresolver-5.3.3-data.jar lib/xmlresolver-data.jar b0c487ad2f3e558be8d829c916d2458d10aca6a5bafa7a4d0524b70845e48a5c; \
    fetch $M/name/dmaus/schxslt/schxslt/1.10.1/schxslt-1.10.1.jar /tmp/schxslt.jar 4f4f21edab7b37f96ad59ae12a344d3510f1092ac46b6d81a4efa0120b73cb58; \
    fetch $R/CEN-EN16931-UBL.sch rules/CEN-EN16931-UBL.sch 268d4f7a2688676695e6c69cba6fba69a6802604fee12cb544a6b30ff09555a3; \
    fetch $R/PEPPOL-EN16931-UBL.sch rules/PEPPOL-EN16931-UBL.sch f637444fab75a6da7e5004f94a6de8e71383ef5658dafe6c68925103b17ea148; \
    for r in CEN-EN16931-UBL PEPPOL-EN16931-UBL; do \
      java -cp 'lib/*' net.sf.saxon.Transform -xsl:'jar:file:/tmp/schxslt.jar!/xslt/2.0/pipeline-for-svrl.xsl' -s:rules/$r.sch -o:rules/$r.xsl; \
    done; \
    rm /tmp/schxslt.jar

ENV CLASSPATH=/validator/lib/*
COPY validate.sh /validator/
ENTRYPOINT ["sh", "/validator/validate.sh"]

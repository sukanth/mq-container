/*
© Copyright IBM Corporation 2018

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func configureWebTLS(cms *KeyStore) error {
	dir := "/run/runmqdevserver/tls"
	ks := NewJKSKeyStore(filepath.Join(dir, "key.jks"), cms.Password)
	ts := NewJKSKeyStore(filepath.Join(dir, "trust.jks"), cms.Password)

	log.Debug("Creating key store")
	err := ks.Create()
	if err != nil {
		return err
	}
	log.Debug("Creating trust store")
	err = ts.Create()
	if err != nil {
		return err
	}
	log.Debug("Importing keys")
	err = ks.Import(cms.Filename, cms.Password)
	if err != nil {
		return err
	}

	webConfigDir := "/etc/mqm/web/installations/Installation1/servers/mqweb"
	tlsConfig := filepath.Join(webConfigDir, "tls.xml")
	newTLSConfig := filepath.Join(webConfigDir, "tls-dev.xml")
	err = os.Remove(tlsConfig)
	if err != nil {
		return err
	}
	err = os.Rename(newTLSConfig, tlsConfig)
	if err != nil {
		return err
	}

	return nil
}

func configureTLS(qmName string, inputFile string, passPhrase string) error {
	log.Debug("Configuring TLS")

	_, err := os.Stat(inputFile)
	if err != nil {
		return err
	}

	// TODO: Use a persisted file (on the volume) instead?
	dir := "/run/runmqdevserver/tls"
	keyFile := filepath.Join(dir, "key.kdb")

	_, err = os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0770)
			if err != nil {
				return err
			}
			err = os.Chown(dir, 999, 999)
			if err != nil {
				log.Debug(err)
				return err
			}
		} else {
			return err
		}
	}

	cms := NewCMSKeyStore(keyFile, passPhrase)

	err = cms.Create()
	if err != nil {
		return err
	}

	err = cms.CreateStash()
	if err != nil {
		return err
	}

	err = cms.Import(inputFile, passPhrase)
	if err != nil {
		return err
	}

	labels, err := cms.GetCertificateLabels()
	if err != nil {
		return err
	}
	if len(labels) == 0 {
		return fmt.Errorf("unable to find certificate label")
	}
	log.Debugf("Renaming certificate from %v", labels[0])
	const newLabel string = "devcert"
	err = cms.RenameCertificate(labels[0], newLabel)
	if err != nil {
		return err
	}

	f, err := os.OpenFile("/etc/mqm/20-dev-tls.mqsc", os.O_WRONLY|os.O_CREATE, 0770)
	if err != nil {
		return err
	}
	defer f.Close()
	// Change the Queue Manager's Key Repository to point at the new TLS key store
	fmt.Fprintf(f, "ALTER QMGR SSLKEYR('%s')\n", filepath.Join(dir, "key"))
	fmt.Fprintf(f, "ALTER QMGR CERTLABL('%s')\n", newLabel)

	if os.Getenv("MQ_DEV") == "true" {
		// Alter the DEV channels to use TLS
		fmt.Fprintln(f, "ALTER CHANNEL('DEV.APP.SVRCONN') CHLTYPE(SVRCONN) SSLCIPH(TLS_RSA_WITH_AES_128_CBC_SHA256) SSLCAUTH(OPTIONAL)")
		fmt.Fprintln(f, "ALTER CHANNEL('DEV.ADMIN.SVRCONN') CHLTYPE(SVRCONN) SSLCIPH(TLS_RSA_WITH_AES_128_CBC_SHA256) SSLCAUTH(OPTIONAL)")
	}

	err = configureWebTLS(cms)
	if err != nil {
		return err
	}

	return nil
}
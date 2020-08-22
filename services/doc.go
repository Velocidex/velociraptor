/*

  Velociraptor has a micro-services architecture, even though it is a
  single binary. Within the binary there are multiple services running
  that help perform various tasks.

  Some of these services contain internal state, (e.g. caches) and are
  used to mediate access to those resources.

  Other services are simply libraries, exporting functions. Being in a
  service makes it easy to use these from anywhere without worrying
  about circular imports.
*/

package services

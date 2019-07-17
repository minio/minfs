%define		tag     RELEASE.2017-02-26T20-20-56Z
%define		subver	%(echo %{tag} | sed -e 's/[^0-9]//g')
# git fetch https://github.com/minio/minfs.git refs/tags/RELEASE.2017-02-26T20-20-56Z
# git rev-list -n 1 FETCH_HEAD
%define         commitid ab47fd9801140eea4444cdf28fbe68f7e4a33ceb

##-----------------------------------------------------------------------------
## All package definitions should be placed here in alphabetical order
##
Summary:          MinFS is a fuse driver for S3 compatible object storage.
Name:             minfs
Version:          0.0.%{subver}
Release:          1
Vendor:           MinIO, Inc.
Group:            Applications/File
License:          Apache v2.0
Source0:	  https://github.com/minio/minfs/archive/%{tag}.tar.gz
BuildRoot:	  %{tmpdir}/%{name}-%{version}-root-%(id -u -n)
Requires:         fuse
BuildRequires:    fuse-devel
BuildRequires:    golang >= 1.7.4

## Disable debug packages.
%define         debug_package %{nil}

## Go related tags.
%define		gobuild(o:) go build -ldflags "${LDFLAGS:-}" %{?**};
%define		gopath		%{_libdir}/golang
%define		import_path	github.com/minio/minfs

%description
MinFS is a fuse driver for Amazon S3 compatible object storage server.
MinFS lets you mount a remote bucket (from a S3 compatible object store),
as if it were a local directory. This allows you to read and write from
the remote bucket just by operating on the local mount directory.

%prep
%setup -qc
mv %{name}-*/* .

install -d src/$(dirname %{import_path})
ln -s ../../.. src/%{import_path}

%build
export GOPATH=$(pwd)

# setup flags like 'go run buildscripts/gen-ldflags.go' would do
tag=%{tag}
version=${tag#RELEASE.}
commitid=%{commitid}
scommitid=$(echo $commitid | cut -c1-12)
prefix=%{import_path}/cmd

LDFLAGS="
-X $prefix.Version=$version
-X $prefix.ReleaseTag=$tag
-X $prefix.CommitID=$commitid
-X $prefix.ShortCommitID=$scommitid
"

%gobuild -o %{name}

%install
rm -rf $RPM_BUILD_ROOT
install -d $RPM_BUILD_ROOT%{_sbindir}
install -d $RPM_BUILD_ROOT%{_sysconfdir}/minfs
install -d $RPM_BUILD_ROOT%{_sysconfdir}/minfs/db
install -p %{name} $RPM_BUILD_ROOT%{_sbindir}
install -p mount.minfs $RPM_BUILD_ROOT%{_sbindir}
install -d $RPM_BUILD_ROOT%{_mandir}/man8
install -m 644 docs/minfs.8 $RPM_BUILD_ROOT%{_mandir}/man8
install -m 644 docs/mount.minfs.8 $RPM_BUILD_ROOT%{_mandir}/man8

%clean
rm -rf $RPM_BUILD_ROOT

%files
%defattr(644,root,root,755)
%doc *.md
%{_mandir}/man8/*minfs*.8*
%attr(755,root,root) %{_sbindir}/minfs
%attr(755,root,root) %{_sbindir}/mount.minfs

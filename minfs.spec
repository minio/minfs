%define		tag	RELEASE.2016-10-04T19-44-43Z
%define		subver	%(echo %{tag} | sed -e 's/[^0-9]//g')
# git fetch https://github.com/minio/minfs.git refs/tags/RELEASE.2016-10-04T19-44-43Z
# git rev-list -n 1 FETCH_HEAD
%define         commitid c88fb0f2eda862b424347728c9bfc00dc17c33c1

##-----------------------------------------------------------------------------
## All package definitions should be placed here in alphabetical order
##
Summary:          MinFS is a fuse driver.
Name:             minfs
Version:          0.0.%{subver}
Release:          1
Vendor:           Minio, Inc.
Group:		  Development/Building
License:          Apache v2.0
Source0:	  https://github.com/minio/minfs/archive/%{tag}.tar.gz
BuildRoot:	  %{tmpdir}/%{name}-%{version}-root-%(id -u -n)


## Disable debug packages.
%define         debug_package %{nil}

## Go related tags.
%define		gobuild(o:) go build -ldflags "${LDFLAGS:-}" %{?**};
%define		gopath		%{_libdir}/golang
%define		import_path	github.com/minio/minfs

%description
MinFS is a fuse driver for Amazon S3 compatible object storage server.
Use it to store photos, videos, VMs, containers, log files, or any
blob of data as objects on your object storage server.

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

%clean
rm -rf $RPM_BUILD_ROOT

%files
%defattr(644,root,root,755)
%doc *.md
%attr(755,root,root) %{_sbindir}/minfs
%attr(755,root,root) %{_sbindir}/mount.minfs

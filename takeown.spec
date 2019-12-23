%define ver 0.0.9
%define rel 1%{?dist}

Summary:        A tool to delegate file ownership to non-administrators
Name:           takeown
Vendor:         Manuel Amador (Rudd-O)
Version:        %ver
Release:        %rel
License:        GPL
Group:          System administration tools
Source:         %{name}-%ver.tar.gz
URL:            https://github.com/Rudd-O/takeown
BuildRequires:  golang, python-unversioned-command

%package kde
Summary:        Context menus for KDE file managers to run takeown
Requires:       kde-filesystem, zenity, python, %{name}

%package gnome
Summary:        Context menus for the Nautilus file manager to run takeown
Requires:       nautilus-python, pygobject3, %{name}

%description
takeown is a simple command-line tool that allows non-administrators to take
ownership of files they do not own, so long as the administrator has set
appropriate policy to allow that.

%description kde
This package provides a context menu for KDE file managers to invoke
takeown.

%description gnome
This package provides a context menu for the GNOME file manager Nautilus
to invoke takeown.

%prep
%autosetup

%build
make

%install
make install DESTDIR=$RPM_BUILD_ROOT BINDIR=%{_bindir} DATADIR=%{_datadir}

%files
%defattr(-,root,root)
%attr(4755, root, root) %{_bindir}/%{name}

%files kde
%defattr(-,root,root)
%{_datadir}/kde4/services/ServiceMenus/%{name}.desktop

%files gnome
%defattr(-,root,root)
%{_datadir}/nautilus-python/extensions/%{name}.py*

%changelog
* Sun Jun 06 2015 Manuel Amador <rudd-o@rudd-o.com> 0.0.7-1
- Added dependency on pygobject3 for proper operation

* Sun Jun 06 2015 Manuel Amador <rudd-o@rudd-o.com> 0.0.5-1
- Added support for different distro releases

* Sun Jun 06 2015 Manuel Amador <rudd-o@rudd-o.com> 0.0.4-1
- Added support for Nautilus

* Sun Mar 08 2015 Manuel Amador <rudd-o@rudd-o.com> 0.0.1-1
- First initial release

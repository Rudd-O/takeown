%define ver 0.1.0
%define rel 1%{?dist}

# Work around issue in F35.
%define debug_package %{nil}

Summary:        A tool to delegate file ownership to non-administrators
Name:           takeown
Vendor:         Manuel Amador (Rudd-O)
Version:        %ver
Release:        %rel
License:        GPL
Group:          System administration tools
Source:         %{name}-%ver.tar.gz
URL:            https://github.com/Rudd-O/takeown
BuildRequires:  golang, python3

%package kde
Summary:        Context menus for KDE file managers to run takeown
Requires:       kde-filesystem, kf5-filesystem, zenity, python3, %{name}

%package gnome
Summary:        Context menus for the Nautilus file manager to run takeown
Requires:       nautilus-extensions, python3, gobject-introspection, %{name}

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
%{_datadir}/kservices5/ServiceMenus/%{name}.desktop

%files gnome
%defattr(-,root,root)
%{_datadir}/nautilus-python/extensions/%{name}.py*

%changelog
* Tue May 18 2021 Manuel Amador <rudd-o@rudd-o.com> 0.0.13-1
- Fix changelog

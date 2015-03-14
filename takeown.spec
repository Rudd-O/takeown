%define ver 0.0.2
%define rel 1

Summary:        A tool to delegate file ownership to non-administrators
Name:           takeown
Vendor:         Manuel Amador (Rudd-O)
Version:        %ver
Release:        %rel
License:        GPL
Group:          System administration tools
Source:         %{name}-%ver.tar.xz
URL:            https://github.com/Rudd-O/takeown
BuildRequires:  golang, python

%package kde
Summary:        Context menus for KDE file managers to run takeown
Requires:       kde-filesystem, zenity, python, %{name}

%description
takeown is a simple command-line tool that allows non-administrators to take
ownership of files they do not own, so long as the administrator has set
appropriate policy to allow that.

%description kde
This package provides a context menu for KDE file managers to invoke
takeown.

%prep
%autosetup -n %{name}

%build
make

%install
mkdir -p $RPM_BUILD_ROOT%{_bindir}
cp -f takeown $RPM_BUILD_ROOT%{_bindir}/
chmod 755 $RPM_BUILD_ROOT%{_bindir}

mkdir -p $RPM_BUILD_ROOT%{_datadir}/kde4/services/ServiceMenus
cp -f takeown.desktop $RPM_BUILD_ROOT%{_datadir}/kde4/services/ServiceMenus/
chmod 644 $RPM_BUILD_ROOT%{_datadir}/kde4/services/ServiceMenus/*

%files
%defattr(-,root,root)
%attr(4755, root, root) %{_bindir}/%{name}

%files kde
%defattr(-,root,root)
%{_datadir}/kde4/services/ServiceMenus/%{name}.desktop

%changelog
* Sun Mar 08 2015 Manuel Amador <rudd-o@rudd-o.com> 0.0.1-1
- First initial release

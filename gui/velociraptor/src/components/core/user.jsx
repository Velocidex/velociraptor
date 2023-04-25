import _ from 'lodash';
import React from 'react';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import PropTypes from 'prop-types';

const UserConfig = React.createContext({
    traits: {},
    updateTraits: () => {},
});

const POLL_TIME = 5000;

// A component which maintains the user settings
export class UserSettings extends React.Component {
    static propTypes = {
        children: PropTypes.node,
    }

    updateTraits = () => {
        api.get("v1/GetUserUITraits", {}, this.source.token).then((response) => {
            let traits = response.data.interface_traits;
            if (_.isUndefined(traits)) {
                return;
            }

            if (traits.lang) {
                window.globals.lang = traits.lang;
            }

            if (traits.org) {
                window.globals.OrgId = traits.org;
            }

            if (traits.auth_redirect_template) {
                window.globals.AuthRedirectTemplate = traits.auth_redirect_template;
            }

            traits.username = response.data.username;
            traits.orgs = response.data.orgs;

            this.setState({traits: traits});

            document.body.classList.remove('no-theme');
            document.body.classList.remove('veloci-dark');
            document.body.classList.remove('veloci-light');
            document.body.classList.remove('pink-light');
            document.body.classList.remove('github-dimmed-dark');
            document.body.classList.remove('github-dimmed-light');
            document.body.classList.remove('ncurses');
            document.body.classList.remove('coolgray-dark');
            document.body.classList.remove('midnight');
            document.body.classList.add(traits.theme || "veloci-light");
        });
    }

    state = {
        traits: {},
        updateTraits: this.updateTraits,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.interval = setInterval(this.updateTraits, POLL_TIME);
        this.updateTraits();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
        clearInterval(this.interval);
    }

    getUserOptions = (traits) => {
        return JSON.parse(traits.ui_settings || "{}");
    }

    render() {
        return (
            <UserConfig.Provider value={this.state}>
              { this.props.children }
            </UserConfig.Provider>
        );
    }
};

export default UserConfig;

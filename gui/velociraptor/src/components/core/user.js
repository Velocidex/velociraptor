import _ from 'lodash';
import React from 'react';
import api from '../core/api-service.js';
import axios from 'axios';

const UserConfig = React.createContext({
    traits: {},
    updateTraits: () => {},
});

const POLL_TIME = 5000;

// A component which maintains the user settings
export class UserSettings extends React.Component {
    updateTraits = () => {
        api.get("v1/GetUserUITraits", {}, this.source.token).then((response) => {
            let traits = response.data.interface_traits;
            if (_.isUndefined(traits)) {
                return;
            }

            traits.username = response.data.username;
            this.setState({traits: traits});

            document.body.classList.remove('no-theme');
            document.body.classList.remove('veloci-dark');
            document.body.classList.remove('veloci-light');
            document.body.classList.remove('pink-light');
            document.body.classList.remove('github-dimmed-dark');
            document.body.classList.remove('github-dimmed-light');
            // document.body.classList.remove('ncurses');
            document.body.classList.remove('coolgray-dark');
            document.body.classList.add(traits.theme || "veloci-light");
        });
    }

    state = {
        traits: {},
        updateTraits: this.updateTraits,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
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

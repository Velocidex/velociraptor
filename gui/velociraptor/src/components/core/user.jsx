import _ from 'lodash';
import React from 'react';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import PropTypes from 'prop-types';
import qs from "qs";
import { withRouter }  from "react-router-dom";
import { JSONparse } from '../utils/json_parse.jsx';

const UserConfig = React.createContext({
    traits: {},
    updateTraits: () => {},
});

const POLL_TIME = 5000;

// A component which maintains the user settings
class _UserSettings extends React.Component {
    static propTypes = {
        children: PropTypes.node,
        history: PropTypes.object,
    }

    updateTraits = () => {
        this.source.cancel("unmounted");
        this.source = CancelToken.source();
        api.get("v1/GetUserUITraits", {},
                this.source.token).then((response) => {
                    let traits = response.data.interface_traits;
                    if (_.isUndefined(traits)) {
                        return;
                    }

                    if (traits.lang) {
                        window.globals.lang = traits.lang;
                    }

                    if (traits.org) {
                        // Get the org id from the url if possible. If it is
                        // not in the URL, get the default from the user
                        // traits.
                        let search = window.location.search.replace('?', '');
                        let params = qs.parse(search);
                        let org_id = params.org_id || traits.org;

                        window.globals.OrgId = org_id;

                        // If the URL does not specify an org, then we need to
                        // set it to the default org from the traits.
                        if (_.isEmpty(params.org_id)) {
                            window.history.pushState({}, "", api.href("/app/index.html", {}));
                            this.props.history.replace("/welcome");
                        }
                    }

                    if (traits.auth_redirect_template) {
                        window.globals.AuthRedirectTemplate = traits.auth_redirect_template;
                    }

                    traits.username = response.data.username;
                    traits.orgs = response.data.orgs;

                    let current_theme = this.state.traits.theme;

                    this.setState({
                        traits: traits,
                        global_messages: response.data.global_messages || []});

                    // Only upate the theme if it changed.
                    if (current_theme !== traits.theme) {
                        document.body.classList.remove('no-theme');
                        document.body.classList.remove('veloci-dark');
                        document.body.classList.remove('veloci-light');
                        document.body.classList.remove('veloci-docs');
                        document.body.classList.remove('pink-light');
                        document.body.classList.remove('github-dimmed-dark');
                        document.body.classList.remove('ncurses-light');
                        document.body.classList.remove('ncurses-dark');
                        document.body.classList.remove('coolgray-dark');
                        document.body.classList.remove('midnight');
                        document.body.classList.remove('vscode-dark');
                        document.body.classList.add(traits.theme || "veloci-light");

                        // veloci-docs is just a modified version of veloci-light
                        if (traits.theme === "veloci-docs") {
                            document.body.classList.add("veloci-light");
                        }
                    }
                });
    }

    state = {
        traits: {},
        global_messages: [],
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
        return JSONparse(traits.ui_settings, {});
    }

    render() {
        return (
            <UserConfig.Provider value={this.state}>
              { this.props.children }
            </UserConfig.Provider>
        );
    }
};

export const UserSettings = withRouter(_UserSettings);

export default UserConfig;
